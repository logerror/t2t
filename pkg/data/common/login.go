package common

import (
	"github.com/golang-jwt/jwt/v5"
)

type LoginInput struct {
	Version  string
	User     string
	Password string
}

type LoginOutput struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

type AuthClaims struct {
	jwt.RegisteredClaims
}
