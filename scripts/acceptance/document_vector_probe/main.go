package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type vectorProbeResult struct {
	FileMD5          string `json:"fileMd5"`
	ChunkCount       int    `json:"chunkCount"`
	ChunkIDs         []int  `json:"chunkIds"`
	Contiguous       bool   `json:"contiguous"`
	HasDuplicate     bool   `json:"hasDuplicate"`
	UserID           uint   `json:"userId"`
	OrgTag           string `json:"orgTag"`
	IsPublic         bool   `json:"isPublic"`
	Signature        string `json:"signature"`
	FirstTextPreview string `json:"firstTextPreview"`
	WaitedMs         int64  `json:"waitedMs"`
}

type vectorRow struct {
	ChunkID     int
	TextContent string
	UserID      uint
	OrgTag      sql.NullString
	IsPublic    bool
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func queryVectors(db *sql.DB, fileMD5 string) ([]vectorRow, error) {
	rows, err := db.Query(`
SELECT chunk_id, text_content, user_id, org_tag, is_public
FROM document_vectors
WHERE file_md5 = ?
ORDER BY chunk_id ASC
`, fileMD5)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]vectorRow, 0)
	for rows.Next() {
		var row vectorRow
		if err := rows.Scan(&row.ChunkID, &row.TextContent, &row.UserID, &row.OrgTag, &row.IsPublic); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func buildResult(fileMD5 string, rows []vectorRow, waited time.Duration) vectorProbeResult {
	res := vectorProbeResult{
		FileMD5:  fileMD5,
		ChunkIDs: make([]int, 0, len(rows)),
		WaitedMs: waited.Milliseconds(),
	}
	if len(rows) == 0 {
		res.Contiguous = true
		res.Signature = sha256Hex("")
		return res
	}

	res.ChunkCount = len(rows)
	res.UserID = rows[0].UserID
	res.OrgTag = rows[0].OrgTag.String
	res.IsPublic = rows[0].IsPublic
	res.FirstTextPreview = firstPreview(rows[0].TextContent, 80)

	hasher := sha256.New()
	expectedChunkID := rows[0].ChunkID
	seen := make(map[int]struct{}, len(rows))
	res.Contiguous = true

	for idx, row := range rows {
		res.ChunkIDs = append(res.ChunkIDs, row.ChunkID)
		if _, ok := seen[row.ChunkID]; ok {
			res.HasDuplicate = true
		}
		seen[row.ChunkID] = struct{}{}

		if idx == 0 {
			expectedChunkID = row.ChunkID
		}
		if row.ChunkID != expectedChunkID {
			res.Contiguous = false
		}
		expectedChunkID++

		_, _ = hasher.Write([]byte(fmt.Sprintf("%d:%s\n", row.ChunkID, row.TextContent)))
	}

	res.Signature = hex.EncodeToString(hasher.Sum(nil))
	return res
}

func firstPreview(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes])
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func main() {
	mysqlDSN := flag.String("mysql-dsn", "root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True", "MySQL DSN")
	fileMD5 := flag.String("file-md5", "", "File MD5")
	expectedMinChunks := flag.Int("expected-min-chunks", 1, "Expected minimum chunks")
	timeoutSec := flag.Int("timeout-sec", 25, "Wait timeout in seconds")
	pollMS := flag.Int("poll-ms", 500, "Poll interval in milliseconds")
	flag.Parse()

	if strings.TrimSpace(*fileMD5) == "" {
		fmt.Fprintln(os.Stderr, "file-md5 is required")
		os.Exit(1)
	}

	db, err := sql.Open("mysql", *mysqlDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open mysql failed: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "mysql ping failed: %v\n", err)
		os.Exit(1)
	}

	timeout := time.Duration(*timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	interval := time.Duration(*pollMS) * time.Millisecond
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	start := time.Now()
	var last vectorProbeResult
	for {
		rows, err := queryVectors(db, *fileMD5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "query document_vectors failed: %v\n", err)
			os.Exit(1)
		}
		last = buildResult(*fileMD5, rows, time.Since(start))

		if last.ChunkCount >= *expectedMinChunks {
			printJSON(last)
			os.Exit(0)
		}
		if time.Since(start) >= timeout {
			printJSON(last)
			os.Exit(2)
		}
		time.Sleep(interval)
	}
}
