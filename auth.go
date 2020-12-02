package gemdrive

import ()

type Auth struct {
	metaRoot string
}

func NewAuth(metaRoot string) *Auth {
	return &Auth{metaRoot}
}

func (a *Auth) CanRead(token, path string) bool {
	return true
}
