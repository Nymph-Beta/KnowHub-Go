package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/internal/service"
	applog "pai_smart_go_v2/pkg/log"

	"github.com/gin-gonic/gin"
)

type fakeUserService struct {
	registerFn             func(username, password string) (*model.User, error)
	loginFn                func(username, password string) (string, string, error)
	getProfileFn           func(username string) (*model.User, error)
	logoutFn               func(token string) error
	setUserPrimaryOrgFn    func(userID uint, orgTagID string) error
	getUserOrgTagsFn       func(userID uint) ([]model.OrganizationTag, error)
	getUserEffectiveTagsFn func(userID uint) ([]model.OrganizationTag, error)
}

func (f *fakeUserService) Register(username, password string) (*model.User, error) {
	if f.registerFn != nil {
		return f.registerFn(username, password)
	}
	return nil, nil
}

func (f *fakeUserService) Login(username, password string) (string, string, error) {
	if f.loginFn != nil {
		return f.loginFn(username, password)
	}
	return "", "", nil
}

func (f *fakeUserService) GetProfile(username string) (*model.User, error) {
	if f.getProfileFn != nil {
		return f.getProfileFn(username)
	}
	return nil, nil
}

func (f *fakeUserService) Logout(token string) error {
	if f.logoutFn != nil {
		return f.logoutFn(token)
	}
	return nil
}

func (f *fakeUserService) SetUserPrimaryOrg(userID uint, orgTagID string) error {
	if f.setUserPrimaryOrgFn != nil {
		return f.setUserPrimaryOrgFn(userID, orgTagID)
	}
	return nil
}

func (f *fakeUserService) GetUserOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if f.getUserOrgTagsFn != nil {
		return f.getUserOrgTagsFn(userID)
	}
	return []model.OrganizationTag{}, nil
}

func (f *fakeUserService) GetUserEffectiveOrgTags(userID uint) ([]model.OrganizationTag, error) {
	if f.getUserEffectiveTagsFn != nil {
		return f.getUserEffectiveTagsFn(userID)
	}
	return []model.OrganizationTag{}, nil
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	applog.Init("error", "console", "")
	m.Run()
}

func newRouter(h *UserHandler) *gin.Engine {
	r := gin.New()
	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	r.GET("/profile", h.GetProfile)
	return r
}

func doReq(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRegister_Success(t *testing.T) {
	svc := &fakeUserService{
		registerFn: func(username, password string) (*model.User, error) {
			return &model.User{
				ID:        1,
				Username:  username,
				Role:      "USER",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/register", `{"username":"alice","password":"123456"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expect 201, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRegister_InvalidBody(t *testing.T) {
	svc := &fakeUserService{}
	r := newRouter(NewUserHandler(svc))

	// 缺少 password 字段
	w := doReq(r, http.MethodPost, "/register", `{"username":"alice"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}

	// 空 JSON
	w = doReq(r, http.MethodPost, "/register", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400 for empty body, got %d, body=%s", w.Code, w.Body.String())
	}

	// 非法 JSON
	w = doReq(r, http.MethodPost, "/register", `not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400 for invalid json, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRegister_UserExists(t *testing.T) {
	svc := &fakeUserService{
		registerFn: func(username, password string) (*model.User, error) {
			return nil, service.ErrUserAlreadyExists
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/register", `{"username":"alice","password":"123456"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expect 409, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRegister_InternalError(t *testing.T) {
	svc := &fakeUserService{
		registerFn: func(username, password string) (*model.User, error) {
			return nil, service.ErrInternal
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/register", `{"username":"alice","password":"123456"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expect 500, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_InvalidBody(t *testing.T) {
	svc := &fakeUserService{}
	r := newRouter(NewUserHandler(svc))

	// 缺少 password 字段
	w := doReq(r, http.MethodPost, "/login", `{"username":"alice"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d, body=%s", w.Code, w.Body.String())
	}

	// 非法 JSON
	w = doReq(r, http.MethodPost, "/login", `not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expect 400 for invalid json, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	svc := &fakeUserService{
		loginFn: func(username, password string) (string, string, error) {
			return "", "", service.ErrInvalidCredentials
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/login", `{"username":"alice","password":"wrong"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_InternalError(t *testing.T) {
	svc := &fakeUserService{
		loginFn: func(username, password string) (string, string, error) {
			return "", "", service.ErrInternal
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/login", `{"username":"alice","password":"123456"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expect 500, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_Success(t *testing.T) {
	svc := &fakeUserService{
		loginFn: func(username, password string) (string, string, error) {
			return "access-token", "refresh-token", nil
		},
	}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodPost, "/login", `{"username":"alice","password":"123456"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "Login successful" {
		t.Fatalf("unexpected message: %v", resp["message"])
	}

	// 验证响应体中包含 token
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expect data to be map, got %T", resp["data"])
	}
	if data["accessToken"] != "access-token" {
		t.Fatalf("expect accessToken='access-token', got %v", data["accessToken"])
	}
	if data["refreshToken"] != "refresh-token" {
		t.Fatalf("expect refreshToken='refresh-token', got %v", data["refreshToken"])
	}
}

func TestGetProfile_NoUserInContext(t *testing.T) {
	svc := &fakeUserService{}
	r := newRouter(NewUserHandler(svc))

	w := doReq(r, http.MethodGet, "/profile", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestGetProfile_InvalidTypeInContext(t *testing.T) {
	svc := &fakeUserService{}
	h := NewUserHandler(svc)

	r := gin.New()
	r.GET("/profile", func(c *gin.Context) {
		// 注入一个非 *model.User 类型的值，触发类型断言失败
		c.Set("user", "not-a-user-struct")
		h.GetProfile(c)
	})

	w := doReq(r, http.MethodGet, "/profile", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expect 500, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestGetProfile_Success(t *testing.T) {
	svc := &fakeUserService{}
	h := NewUserHandler(svc)

	r := gin.New()
	r.GET("/profile", func(c *gin.Context) {
		c.Set("user", &model.User{
			ID:        7,
			Username:  "alice",
			Role:      "USER",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		h.GetProfile(c)
	})

	w := doReq(r, http.MethodGet, "/profile", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestMapServiceError_UserNotFound(t *testing.T) {
	status, msg := mapServiceError(service.ErrUserNotFound)
	if status != http.StatusNotFound || msg != "User not found" {
		t.Fatalf("expect 404 'User not found', got %d %q", status, msg)
	}
}

func TestMapServiceError_Default500(t *testing.T) {
	status, msg := mapServiceError(errors.New("unknown"))
	if status != http.StatusInternalServerError || msg != "Internal server error" {
		t.Fatalf("unexpected map result: %d %s", status, msg)
	}
}
