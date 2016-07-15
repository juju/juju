package identityservice

import (
	"fmt"
	"strconv"
)

type Users struct {
	nextUserId   int
	nextTenantId int
	users        map[string]UserInfo
	tenants      map[string]string
}

func (u *Users) addTenant(tenant string) string {
	for id, tenantName := range u.tenants {
		if tenant == tenantName {
			return id
		}
	}
	u.nextTenantId++
	id := strconv.Itoa(u.nextTenantId)
	u.tenants[id] = tenant
	return id
}

func (u *Users) AddUser(user, secret, tenant string) *UserInfo {
	tenantId := u.addTenant(tenant)
	u.nextUserId++
	userInfo := &UserInfo{secret: secret, Id: strconv.Itoa(u.nextUserId), TenantId: tenantId}
	u.users[user] = *userInfo
	userInfo, _ = u.authenticate(user, secret)
	return userInfo
}

func (u *Users) FindUser(token string) (*UserInfo, error) {
	for _, userInfo := range u.users {
		if userInfo.Token == token {
			return &userInfo, nil
		}
	}
	return nil, fmt.Errorf("No user with token %v exists", token)
}

const (
	notAuthorized = "The request you have made requires authentication."
	invalidUser   = "Invalid user / password"
)

func (u *Users) authenticate(username, password string) (*UserInfo, string) {
	userInfo, ok := u.users[username]
	if !ok {
		return nil, notAuthorized
	}
	if userInfo.secret != password {
		return nil, invalidUser
	}
	if userInfo.Token == "" {
		userInfo.Token = randomHexToken()
		u.users[username] = userInfo
	}
	return &userInfo, ""
}
