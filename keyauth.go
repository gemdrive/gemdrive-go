package gemdrive

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"
	"strings"
)

type KeyData struct {
	Parent     string            `json:"parent"`
	Privileges map[string]string `json:"privileges"`
}

func (k KeyData) CanRead(pathStr string) bool {
	for path, perm := range k.Privileges {
		isSubpath := strings.HasPrefix(pathStr, path)
		return isSubpath && permCanRead(perm)
	}
	return false
}
func (k KeyData) CanWrite(pathStr string) bool {
	for path, perm := range k.Privileges {
		isSubpath := strings.HasPrefix(pathStr, path)
		return isSubpath && permCanWrite(perm)
	}
	return false
}
func (k KeyData) IsSubsetOf(other *KeyData) bool {
	for thisPath, thisPerm := range k.Privileges {
		if !k.coveredBy(thisPath, thisPerm, other) {
			return false
		}
	}
	return true
}
func (k KeyData) coveredBy(path, perm string, other *KeyData) bool {
	for otherPath, otherPerm := range other.Privileges {
		isSubpath := strings.HasPrefix(path, otherPath)
		if !isSubpath {
			continue
		}

		if perm == "read" && permCanRead(otherPerm) {
			return true
		}

		if perm == "write" && permCanWrite(otherPerm) {
			return true
		}
	}
	return false
}

type Privilege struct {
	Path string `json:"path"`
	Perm string `json:"perm"`
}

func (p Privilege) CanRead(pathStr string) bool {
	isSubpath := strings.HasPrefix(pathStr, p.Path)
	return isSubpath && permCanRead(p.Perm)
}
func (p Privilege) CanWrite(pathStr string) bool {
	isSubpath := strings.HasPrefix(pathStr, p.Path)
	return isSubpath && permCanWrite(p.Perm)
}

type KeyAuth struct {
	db *GemDriveDatabase
}

func NewKeyAuth(db *GemDriveDatabase) (*KeyAuth, error) {
	return &KeyAuth{
		db: db,
	}, nil
}

func (a *KeyAuth) CanRead(key, pathStr string) bool {

	keyData, err := a.db.GetKeyData(key)
	if err != nil {
		return false
	}

	return keyData.CanRead(pathStr)
}

func (a *KeyAuth) CanWrite(key, pathStr string) bool {

	keyData, err := a.db.GetKeyData(key)
	if err != nil {
		return false
	}

	return keyData.CanWrite(pathStr)
}

func saveJson(data interface{}, filePath string) error {
	jsonStr, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.New("Error serializing JSON")
	} else {
		err := ioutil.WriteFile(filePath, jsonStr, 0644)
		if err != nil {
			return errors.New("Error saving JSON")
		}
	}
	return nil
}

func permCanRead(perm string) bool {
	return perm == "read" || permCanWrite(perm)
}

func permCanWrite(perm string) bool {
	return perm == "write"
}

func genRandomKey() (string, error) {
	const chars string = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	id := ""
	for i := 0; i < 32; i++ {
		randIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		id += string(chars[randIndex.Int64()])
	}
	return id, nil
}
