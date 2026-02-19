package service

import (
	"errors"
	"os"
	"testing"
	"time"

	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/hash"
	applog "pai_smart_go_v2/pkg/log"
	"pai_smart_go_v2/pkg/token"

	"gorm.io/gorm"
)

type fakeUserRepo struct {
	findByUsernameFn     func(username string) (*model.User, error)
	createFn             func(user *model.User) error
	updateFn             func(user *model.User) error
	findAllFn            func() ([]model.User, error)
	findWithPaginationFn func(offset, limit int) ([]model.User, int64, error)
	findByIDFn           func(userID uint) (*model.User, error)
}

func (f *fakeUserRepo) Create(user *model.User) error {
	if f.createFn != nil {
		return f.createFn(user)
	}
	return nil
}
func (f *fakeUserRepo) FindByUsername(username string) (*model.User, error) {
	if f.findByUsernameFn != nil {
		return f.findByUsernameFn(username)
	}
	return nil, nil
}
func (f *fakeUserRepo) Update(user *model.User) error {
	if f.updateFn != nil {
		return f.updateFn(user)
	}
	return nil
}
func (f *fakeUserRepo) FindAll() ([]model.User, error) {
	if f.findAllFn != nil {
		return f.findAllFn()
	}
	return []model.User{}, nil
}
func (f *fakeUserRepo) FindWithPagination(offset, limit int) ([]model.User, int64, error) {
	if f.findWithPaginationFn != nil {
		return f.findWithPaginationFn(offset, limit)
	}
	return []model.User{}, 0, nil
}
func (f *fakeUserRepo) FindByID(userID uint) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(userID)
	}
	return nil, nil
}

type fakeOrgTagRepo struct {
	createFn                    func(tag *model.OrganizationTag) error
	findAllFn                   func() ([]model.OrganizationTag, error)
	findByIDFn                  func(id string) (*model.OrganizationTag, error)
	findByParentTagFn           func(parentTag *string) ([]model.OrganizationTag, error)
	updateFn                    func(tag *model.OrganizationTag) error
	deleteFn                    func(tagID string) error
	deleteAndReparentChildrenFn func(tagID string) error
}

func (f *fakeOrgTagRepo) Create(tag *model.OrganizationTag) error {
	if f.createFn != nil {
		return f.createFn(tag)
	}
	return nil
}

func (f *fakeOrgTagRepo) FindAll() ([]model.OrganizationTag, error) {
	if f.findAllFn != nil {
		return f.findAllFn()
	}
	return []model.OrganizationTag{}, nil
}

func (f *fakeOrgTagRepo) FindByID(id string) (*model.OrganizationTag, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeOrgTagRepo) FindByParentTag(parentTag *string) ([]model.OrganizationTag, error) {
	if f.findByParentTagFn != nil {
		return f.findByParentTagFn(parentTag)
	}
	return []model.OrganizationTag{}, nil
}

func (f *fakeOrgTagRepo) Update(tag *model.OrganizationTag) error {
	if f.updateFn != nil {
		return f.updateFn(tag)
	}
	return nil
}

func (f *fakeOrgTagRepo) Delete(tagID string) error {
	if f.deleteFn != nil {
		return f.deleteFn(tagID)
	}
	return nil
}

func (f *fakeOrgTagRepo) DeleteAndReparentChildren(tagID string) error {
	if f.deleteAndReparentChildrenFn != nil {
		return f.deleteAndReparentChildrenFn(tagID)
	}
	return nil
}

func TestMain(m *testing.M) {
	// service 里有 log.Errorf，初始化一下避免 nil panic
	applog.Init("error", "console", "")
	code := m.Run()
	os.Exit(code)
}

func newJWT() *token.JWTManager {
	return token.NewJWTManager("test-secret", 15*time.Minute, 24*time.Hour)
}

func TestUserService_Register_Success(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		createFn: func(user *model.User) error {
			user.ID = 1
			return nil
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	u, err := svc.Register("alice", "123456")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if u.ID != 1 || u.Username != "alice" || u.Role != "USER" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if u.Password == "123456" || !hash.CheckPasswordHash("123456", u.Password) {
		t.Fatalf("password is not hashed correctly")
	}
}

func TestUserService_Register_UserAlreadyExists(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return &model.User{ID: 1, Username: "alice"}, nil
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, err := svc.Register("alice", "123456")
	if !errors.Is(err, ErrUserAlreadyExists) {
		t.Fatalf("expect ErrUserAlreadyExists, got %v", err)
	}
}

func TestUserService_Login_Success(t *testing.T) {
	pwd, _ := hash.HashPassword("123456")
	jm := newJWT()
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return &model.User{
				ID:       1,
				Username: "alice",
				Password: pwd,
				Role:     "USER",
			}, nil
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, jm)

	access, refresh, err := svc.Login("alice", "123456")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatalf("tokens should not be empty")
	}
	claims, err := jm.VerifyToken(access)
	if err != nil {
		t.Fatalf("VerifyToken(access) error = %v", err)
	}
	if claims.Username != "alice" {
		t.Fatalf("unexpected claims username: %s", claims.Username)
	}
}

func TestUserService_Login_UserNotFound(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, _, err := svc.Login("no-user", "123456")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expect ErrInvalidCredentials, got %v", err)
	}
}

func TestUserService_Login_WrongPassword(t *testing.T) {
	pwd, _ := hash.HashPassword("correct-password")
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return &model.User{
				ID:       1,
				Username: "alice",
				Password: pwd,
				Role:     "USER",
			}, nil
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, _, err := svc.Login("alice", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expect ErrInvalidCredentials for wrong password, got %v", err)
	}
}

func TestUserService_Login_DBError(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, _, err := svc.Login("alice", "123456")
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expect ErrInternal for DB error, got %v", err)
	}
}

func TestUserService_Login_NilJWTManager(t *testing.T) {
	svc := NewUserService(&fakeUserRepo{}, &fakeOrgTagRepo{}, nil)

	_, _, err := svc.Login("alice", "123456")
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expect ErrInternal for nil JWTManager, got %v", err)
	}
}

func TestUserService_Register_DBErrorOnFind(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, err := svc.Register("alice", "123456")
	if err == nil {
		t.Fatalf("expect error for DB failure, got nil")
	}
}

func TestUserService_Register_CreateError(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		createFn: func(user *model.User) error {
			return errors.New("duplicate key")
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, err := svc.Register("alice", "123456")
	if err == nil {
		t.Fatalf("expect error for Create failure, got nil")
	}
}

func TestUserService_GetProfile_Success(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return &model.User{
				ID:       1,
				Username: "alice",
				Role:     "USER",
			}, nil
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	u, err := svc.GetProfile("alice")
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if u.ID != 1 || u.Username != "alice" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestUserService_GetProfile_NotFound(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, err := svc.GetProfile("no-user")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expect ErrUserNotFound, got %v", err)
	}
}

func TestUserService_GetProfile_DBError(t *testing.T) {
	repo := &fakeUserRepo{
		findByUsernameFn: func(username string) (*model.User, error) {
			return nil, errors.New("db down")
		},
	}
	svc := NewUserService(repo, &fakeOrgTagRepo{}, newJWT())

	_, err := svc.GetProfile("alice")
	if !errors.Is(err, ErrInternal) {
		t.Fatalf("expect ErrInternal, got %v", err)
	}
}

func TestUserService_AssignOrgTagsToUser_Success(t *testing.T) {
	updated := &model.User{}
	repo := &fakeUserRepo{
		findByIDFn: func(userID uint) (*model.User, error) {
			return &model.User{
				ID:         userID,
				Username:   "alice",
				Role:       "USER",
				OrgTags:    "legacy",
				PrimaryOrg: "legacy",
			}, nil
		},
		updateFn: func(user *model.User) error {
			*updated = *user
			return nil
		},
	}
	orgRepo := &fakeOrgTagRepo{
		findByIDFn: func(id string) (*model.OrganizationTag, error) {
			return &model.OrganizationTag{TagID: id, Name: id}, nil
		},
	}
	svc := NewUserService(repo, orgRepo, newJWT())

	err := svc.AssignOrgTagsToUser(7, []string{"team-a", " team-b ", "team-a"})
	if err != nil {
		t.Fatalf("AssignOrgTagsToUser() error = %v", err)
	}
	if updated.OrgTags != "team-a,team-b" {
		t.Fatalf("unexpected OrgTags, got %q", updated.OrgTags)
	}
	if updated.PrimaryOrg != "team-a" {
		t.Fatalf("unexpected PrimaryOrg, got %q", updated.PrimaryOrg)
	}
}

func TestUserService_AssignOrgTagsToUser_TagNotFound(t *testing.T) {
	repo := &fakeUserRepo{
		findByIDFn: func(userID uint) (*model.User, error) {
			return &model.User{ID: userID, Username: "alice"}, nil
		},
	}
	orgRepo := &fakeOrgTagRepo{
		findByIDFn: func(id string) (*model.OrganizationTag, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewUserService(repo, orgRepo, newJWT())

	err := svc.AssignOrgTagsToUser(1, []string{"missing-tag"})
	if !errors.Is(err, ErrOrgTagNotFound) {
		t.Fatalf("expect ErrOrgTagNotFound, got %v", err)
	}
}

func TestUserService_ListUsers_Success(t *testing.T) {
	repo := &fakeUserRepo{
		findWithPaginationFn: func(offset, limit int) ([]model.User, int64, error) {
			return []model.User{
				{
					ID:         1,
					Username:   "admin",
					Role:       "ADMIN",
					OrgTags:    "root,team-a",
					PrimaryOrg: "root",
					CreatedAt:  time.Unix(100, 0),
				},
			}, 1, nil
		},
	}
	orgRepo := &fakeOrgTagRepo{
		findAllFn: func() ([]model.OrganizationTag, error) {
			return []model.OrganizationTag{
				{TagID: "root", Name: "Root"},
				{TagID: "team-a", Name: "Team A"},
			}, nil
		},
	}
	svc := NewUserService(repo, orgRepo, newJWT())

	users, total, err := svc.ListUsers(1, 10)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("expect total=1, got %d", total)
	}
	if len(users) != 1 {
		t.Fatalf("expect 1 user, got %d", len(users))
	}
	if users[0].Status != 0 {
		t.Fatalf("expect admin status=0, got %d", users[0].Status)
	}
	if len(users[0].OrgTags) != 2 {
		t.Fatalf("expect 2 org tags, got %d", len(users[0].OrgTags))
	}
}
