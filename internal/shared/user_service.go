package shared

import (
	"github.com/google/uuid"
)

// User implements the UserDataForToken interface.
func (u *User) GetID() uuid.UUID {
	return u.ID
}

func (u *User) GetEmail() *string {
	return u.Email
}

func (u *User) GetRole() string {
	return u.Role
}
