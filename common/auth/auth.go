package auth

import (
	"crypto/subtle"
)

type User struct {
	Username string
	Password string
}

type Authenticator struct {
	userMap map[string][]string
}

func NewAuthenticator(users []User) *Authenticator {
	if len(users) == 0 {
		return nil
	}
	au := &Authenticator{
		userMap: make(map[string][]string),
	}
	for _, user := range users {
		au.userMap[user.Username] = append(au.userMap[user.Username], user.Password)
	}
	return au
}

func (au *Authenticator) Verify(username string, password string) bool {
	passwordList, ok := au.userMap[username]
	if !ok {
		return false
	}
	matched := 0
	for _, expected := range passwordList {
		matched |= subtle.ConstantTimeCompare([]byte(expected), []byte(password))
	}
	return matched == 1
}
