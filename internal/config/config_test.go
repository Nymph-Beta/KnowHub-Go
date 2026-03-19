package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptTemplate(t *testing.T) {
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "chat.tmpl")
	if err := os.WriteFile(promptPath, []byte("hello {{.References}}"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := &LLMConfig{
		Prompt: LLMPromptConfig{
			TemplateFile: "chat.tmpl",
		},
	}

	if err := loadPromptTemplate(filepath.Join(tempDir, "config.yaml"), cfg); err != nil {
		t.Fatalf("loadPromptTemplate() error = %v", err)
	}
	if cfg.Prompt.Template != "hello {{.References}}" {
		t.Fatalf("unexpected template content: %q", cfg.Prompt.Template)
	}
}
