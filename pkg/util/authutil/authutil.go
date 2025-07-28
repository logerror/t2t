package authutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/data/common"
)

func GetToken() (string, common.AuthClaims) {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	token, err := os.ReadFile(filepath.Join(home, ".config", "t2t", "auth.token"))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("please login first using your account")
			os.Exit(1)
		}
		panic(err)
	}

	var claims common.AuthClaims
	_, _, err = jwt.NewParser().ParseUnverified(string(token), &claims)
	if err != nil {
		panic(err)
	}
	return string(token), claims
}

func GenToken(username string) (string, error) {
	claims := common.AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "t2t",
			Subject:  username,
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(svcconstants.JWTSecret))
}

func AuthenticateLDAP(ldapURL, baseDN, serviceUser, servicePass, username, password string) (bool, error) {

	return false, nil
}
func VerifyToken(token string) (*common.AuthClaims, error) {
	ptoken, err := jwt.ParseWithClaims(token, &common.AuthClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(svcconstants.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}

	return ptoken.Claims.(*common.AuthClaims), err
}
