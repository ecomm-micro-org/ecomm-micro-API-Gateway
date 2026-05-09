package store

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/risbern21/api_gateway/internal/database"
	"github.com/risbern21/api_gateway/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	database.Setup()
	database.Client().Logger.LogMode(0)
	err := database.Client().AutoMigrate(&models.User{}, &models.Session{})
	if err != nil {
		log.Fatal("unable to automigrate")
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}

// newTestUser returns a fully populated User with randomised unique fields.
// It does NOT persist anything — callers decide whether to save it.
func newTestUser() *models.User {
	n := rand.Intn(1_000_000)
	return &models.User{
		Username: fmt.Sprintf("testuser_%d", n),
		Email:    fmt.Sprintf("testuser_%d@example.com", n),
		Password: "hashed_password",
		Role:     models.Buyer,
		Address:  "123 Test Street",
		Phone:    "5550001234",
	}
}

func newTestSession() *models.Session {
	return &models.Session{
		ID:           uuid.NewString(),
		UserEmail:    "test@example.com",
		RefreshToken: "refresh_token",
		IsRevoked:    false,
		ExpiresAt:    time.Now().Add(3 * time.Minute),
	}
}

type Storer interface {
	AddUser(u *models.User) error
	GetUserByEmail(u *models.User, email string) error
	CreateSession(s *models.Session) error
	GetSession(s *models.Session) error
	RevokeSession(s *models.Session) error
	DeleteSession(s *models.Session) error
}

func newTestStore() Storer {
	return &PGStore{
		db: database.Client(),
	}
}

// seedUser persists a user and fails the test immediately on any error.
func seedUser(t *testing.T, db *gorm.DB) *models.User {
	t.Helper()
	u := newTestUser()
	store := newTestStore()
	require.NoError(t, store.AddUser(u), "seeding user must not fail")
	return u
}

func seedSession(t *testing.T, db *gorm.DB) *models.Session {
	t.Helper()
	s := newTestSession()
	store := newTestStore()
	require.NoError(t, store.CreateSession(s), "seeding session must not fail")
	return s
}

func TestAddUser(t *testing.T) {
	db := database.Client()

	tests := []struct {
		name        string
		build       func(t *testing.T) *models.User
		expectError bool
		validate    func(t *testing.T, u *models.User)
	}{
		{
			name: "persists a valid user and populates ID",
			build: func(t *testing.T) *models.User {
				return newTestUser()
			},
			expectError: false,
			validate: func(t *testing.T, u *models.User) {
				assert.NotEqual(t, uuid.UUID{}, u.ID, "database must assign a non-zero UUID")
			},
		},
		{
			name: "persists buyer role correctly",
			build: func(t *testing.T) *models.User {
				u := newTestUser()
				u.Role = models.Buyer
				return u
			},
			expectError: false,
			validate: func(t *testing.T, u *models.User) {
				fetched := &models.User{}
				require.NoError(t, db.First(fetched, "id = ?", u.ID).Error)
				assert.Equal(t, models.Buyer, fetched.Role)
			},
		},
		{
			name: "persists seller role correctly",
			build: func(t *testing.T) *models.User {
				u := newTestUser()
				u.Role = models.Seller
				return u
			},
			expectError: false,
			validate: func(t *testing.T, u *models.User) {
				fetched := &models.User{}
				require.NoError(t, db.First(fetched, "id = ?", u.ID).Error)
				assert.Equal(t, models.Seller, fetched.Role)
			},
		},
		{
			name: "returns error when username is already taken (unique constraint)",
			build: func(t *testing.T) *models.User {
				existing := seedUser(t, db)

				duplicate := newTestUser()
				duplicate.Username = existing.Username // collide on username
				return duplicate
			},
			expectError: true,
		},
		{
			name: "returns error when email is already taken (unique constraint)",
			build: func(t *testing.T) *models.User {
				existing := seedUser(t, db)

				duplicate := newTestUser()
				duplicate.Email = existing.Email // collide on email
				return duplicate
			},
			expectError: true,
		},
		{
			name: "all supplied fields are stored and retrieved correctly",
			build: func(t *testing.T) *models.User {
				u := newTestUser()
				u.Address = "42 Elm St, Springfield"
				u.Phone = "9998887777"
				return u
			},
			expectError: false,
			validate: func(t *testing.T, u *models.User) {
				fetched := &models.User{}
				require.NoError(t, db.First(fetched, "id = ?", u.ID).Error)
				assert.Equal(t, u.Username, fetched.Username)
				assert.Equal(t, u.Email, fetched.Email)
				assert.Equal(t, u.Password, fetched.Password)
				assert.Equal(t, u.Address, fetched.Address)
				assert.Equal(t, u.Phone, fetched.Phone)
			},
		},
		{
			name: "CreatedAt and UpdatedAt are populated by the database",
			build: func(t *testing.T) *models.User {
				return newTestUser()
			},
			expectError: false,
			validate: func(t *testing.T, u *models.User) {
				fetched := &models.User{}
				require.NoError(t, db.First(fetched, "id = ?", u.ID).Error)
				assert.False(t, fetched.CreatedAt.IsZero(), "CreatedAt must be set")
				assert.False(t, fetched.UpdatedAt.IsZero(), "UpdatedAt must be set")
			},
		},
	}

	store := NewPGStore()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.build(t)
			err := store.AddUser(u)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, u)
			}
		})
	}
}

func TestGetUserByEmail(t *testing.T) {
	db := database.Client()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (email string, seeded *models.User)
		expectError bool
		validate    func(t *testing.T, fetched *models.User, seeded *models.User)
	}{
		{
			name: "returns the correct user for a known email",
			setup: func(t *testing.T) (string, *models.User) {
				u := seedUser(t, db)
				return u.Email, u
			},
			expectError: false,
			validate: func(t *testing.T, fetched *models.User, seeded *models.User) {
				assert.Equal(t, seeded.ID, fetched.ID)
				assert.Equal(t, seeded.Email, fetched.Email)
				assert.Equal(t, seeded.Username, fetched.Username)
			},
		},
		{
			name: "returns all fields for a known email",
			setup: func(t *testing.T) (string, *models.User) {
				u := seedUser(t, db)
				return u.Email, u
			},
			expectError: false,
			validate: func(t *testing.T, fetched *models.User, seeded *models.User) {
				assert.Equal(t, seeded.Password, fetched.Password)
				assert.Equal(t, seeded.Role, fetched.Role)
				assert.Equal(t, seeded.Address, fetched.Address)
				assert.Equal(t, seeded.Phone, fetched.Phone)
			},
		},
		{
			name: "returns gorm.ErrRecordNotFound for a non-existent email",
			setup: func(t *testing.T) (string, *models.User) {
				return "does_not_exist@example.com", nil
			},
			expectError: true,
			validate: func(t *testing.T, fetched *models.User, _ *models.User) {
				// callers in handler.go check for gorm.ErrRecordNotFound specifically
				u := models.NewUser()
				err := u.GetUserByEmail(db, "does_not_exist@example.com")
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
		},
		{
			name: "lookup is case-sensitive — upper-cased email does not match",
			setup: func(t *testing.T) (string, *models.User) {
				u := seedUser(t, db)
				// PostgreSQL text comparison is case-sensitive by default
				return fmt.Sprintf("%s_UPPER", u.Email), nil
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, seeded := tt.setup(t)

			fetched := NewUser()
			err := fetched.GetUserByEmail(db, email)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, fetched, seeded)
			}
		})
	}
}

func TestRoleString(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{
			name:     "Buyer stringifies to 'buyer'",
			role:     Buyer,
			expected: "buyer",
		},
		{
			name:     "Seller stringifies to 'seller'",
			role:     Seller,
			expected: "seller",
		},
		{
			name:     "arbitrary role value is returned as-is",
			role:     Role("admin"),
			expected: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}

func TestCreateSession(t *testing.T) {
	db := database.Client()
	tests := []struct {
		name        string
		build       func(t *testing.T) *Session
		expectError bool
		validate    func(t *testing.T, s *Session)
	}{
		{
			name: "persists valid session and populates session details",
			build: func(t *testing.T) *Session {
				return newTestSession()
			},
			expectError: false,
			validate: func(t *testing.T, s *Session) {
				assert.NotEmpty(t, s.CreatedAt, "gorm must populate CreatedAt")
			},
		},
		{
			name: "should not keep duplicate sessions with same session id",
			build: func(t *testing.T) *Session {
				s := newTestSession()
				// Pre-insert the session so the second save hits a duplicate
				require.NoError(t, s.CreateSession(db))
				// Return a new struct with the same ID to trigger the conflict
				return &Session{
					ID:           s.ID,
					UserEmail:    "other@example.com",
					RefreshToken: uuid.NewString(),
					ExpiresAt:    time.Now().Add(24 * time.Hour),
				}
			},
			expectError: true,
			validate:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build(t)
			err := s.CreateSession(db)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, s)
			}
		})
	}
}

func TestGetSession(t *testing.T) {
	db := database.Client()
	tests := []struct {
		name        string
		build       func(t *testing.T) *Session
		expectError bool
		validate    func(t *testing.T, s *Session)
	}{
		{
			name: "returns session by id",
			build: func(t *testing.T) *Session {
				s := newTestSession()
				require.NoError(t, s.CreateSession(db))
				return &Session{ID: s.ID}
			},
			expectError: false,
			validate: func(t *testing.T, s *Session) {
				assert.Equal(t, "test@example.com", s.UserEmail)
				assert.NotEmpty(t, s.RefreshToken)
				assert.NotEmpty(t, s.CreatedAt)
			},
		},
		{
			name: "returns error for non-existent session id",
			build: func(t *testing.T) *Session {
				return &Session{ID: uuid.NewString()}
			},
			expectError: true,
			validate:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build(t)
			err := s.GetSession(db)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, s)
			}
		})
	}
}

func TestRevokeSession(t *testing.T) {
	db := database.Client()
	tests := []struct {
		name        string
		build       func(t *testing.T) *Session
		expectError bool
		validate    func(t *testing.T, s *Session)
	}{
		{
			name: "marks session as revoked",
			build: func(t *testing.T) *Session {
				s := newTestSession()
				require.NoError(t, s.CreateSession(db))
				return s
			},
			expectError: false,
			validate: func(t *testing.T, s *Session) {
				// Re-fetch from DB to confirm the flag was persisted
				fetched := &Session{ID: s.ID}
				require.NoError(t, fetched.GetSession(db))
				assert.True(t, fetched.IsRevoked, "session must be marked revoked in the database")
			},
		},
		{
			name: "revoking a non-existent session id does not error",
			build: func(t *testing.T) *Session {
				// GORM UPDATE on a missing row affects 0 rows but returns no error
				return &Session{ID: uuid.NewString()}
			},
			expectError: false,
			validate:    nil,
		},
		{
			name: "revoking an already-revoked session is idempotent",
			build: func(t *testing.T) *Session {
				s := newTestSession()
				require.NoError(t, s.CreateSession(db))
				require.NoError(t, s.RevokeSession(db))
				return s
			},
			expectError: false,
			validate: func(t *testing.T, s *Session) {
				fetched := &Session{ID: s.ID}
				require.NoError(t, fetched.GetSession(db))
				assert.True(t, fetched.IsRevoked)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build(t)
			err := s.RevokeSession(db)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, s)
			}
		})
	}
}

func TestDeleteSession(t *testing.T) {
	db := database.Client()
	tests := []struct {
		name        string
		build       func(t *testing.T) *Session
		expectError bool
		validate    func(t *testing.T, s *Session)
	}{
		{
			name: "soft-deletes an existing session",
			build: func(t *testing.T) *Session {
				s := newTestSession()
				require.NoError(t, s.CreateSession(db))
				return s
			},
			expectError: false,
			validate: func(t *testing.T, s *Session) {
				// GetSession uses First which respects soft-delete; should now return not-found
				fetched := &Session{ID: s.ID}
				err := fetched.GetSession(db)
				require.Error(t, err, "deleted session must not be retrievable via GetSession")
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			},
		},
		{
			name: "deleting a non-existent session does not error",
			build: func(t *testing.T) *Session {
				return &Session{ID: uuid.NewString()}
			},
			expectError: false,
			validate:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.build(t)
			err := s.DeleteSession(db)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, s)
			}
		})
	}
}
