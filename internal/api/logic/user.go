package logic

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"ai-start/internal/store"
)

// 用户域错误。
var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrPasswordTooShort   = errors.New("密码长度至少 6 位")
	ErrInvalidToken       = errors.New("令牌无效或已过期")
	ErrUserStoreOffline   = errors.New("用户存储不可用")
)

// UserService 用户域业务逻辑：注册、登录、令牌签发与解析。
type UserService struct {
	mysql     *store.MySQLStore // 用户存储
	jwtSecret []byte            // JWT 签名密钥
	expire    time.Duration     // Token 有效期
}

// NewUserService 创建用户服务。expireHours <= 0 时默认 72 小时。
func NewUserService(mysql *store.MySQLStore, jwtSecret string, expireHours int) *UserService {
	if expireHours <= 0 {
		expireHours = 72
	}
	return &UserService{
		mysql:     mysql,
		jwtSecret: []byte(jwtSecret),
		expire:    time.Duration(expireHours) * time.Hour,
	}
}

// Register 用户注册：校验 → 加密 → 落库，返回用户 ID。
func (s *UserService) Register(username, password string) (uint64, error) {
	if s.mysql == nil {
		return 0, ErrUserStoreOffline
	}
	if len(password) < 6 {
		return 0, ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	user := &store.User{Username: username, PasswordHash: string(hash)}
	if err := s.mysql.CreateUser(user); err != nil {
		return 0, err // 含 store.ErrUserExists
	}
	return user.ID, nil
}

// Login 用户登录：校验密码，签发 JWT。
func (s *UserService) Login(username, password string) (token string, userID uint64, err error) {
	if s.mysql == nil {
		return "", 0, ErrUserStoreOffline
	}
	user, err := s.mysql.GetUserByUsername(username)
	if err != nil || user == nil {
		return "", 0, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return "", 0, ErrInvalidCredentials
	}
	token, err = s.issueToken(user.ID, user.Username)
	if err != nil {
		return "", 0, err
	}
	return token, user.ID, nil
}

// issueToken 签发 JWT（HS256），claims 携带 user_id 与 username。
func (s *UserService) issueToken(userID uint64, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(s.expire).Unix(),
		"iat":      time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

// ParseToken 解析并校验 JWT，返回用户 ID（供鉴权中间件使用）。
func (s *UserService) ParseToken(tokenString string) (uint64, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, ErrInvalidToken
	}
	// JWT 数字 claim 解析为 float64，需转换
	userID, ok := claims["user_id"].(float64)
	if !ok {
		return 0, ErrInvalidToken
	}
	return uint64(userID), nil
}
