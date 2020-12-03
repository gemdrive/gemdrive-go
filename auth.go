package gemdrive

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"path"
	"strings"
	"sync"
)

type Auth struct {
	metaRoot string
	db       *Database
}

type Acl []*AclEntry

func (a Acl) CanRead(id string) bool {
	for _, entry := range a {
		if entry.Id == id && permCanRead(entry.Perm) {
			return true
		}
	}
	return false
}
func (a Acl) CanWrite(id string) bool {
	for _, entry := range a {
		if entry.Id == id && permCanWrite(entry.Perm) {
			return true
		}
	}
	return false
}

type AclEntry struct {
	IdType string `json:"idType"`
	Id     string `json:"id"`
	Perm   string `json:"perm"`
}

type Key struct {
	Path string `json:"path"`
	Id   string `json:"id"`
	Perm string `json:"perm"`
}

func (k Key) CanRead(pathStr string) bool {
	isSubpath := strings.HasPrefix(pathStr, k.Path)
	return isSubpath && permCanRead(k.Perm)
}
func (k Key) CanWrite(pathStr string) bool {
	isSubpath := strings.HasPrefix(pathStr, k.Path)
	return isSubpath && permCanWrite(k.Perm)
}

type Database struct {
	Keys map[string]*Key `json:"keys"`
	mut  *sync.Mutex
}

func NewDatabase() *Database {
	dbJson, err := ioutil.ReadFile("gemdrive_db.json")
	if err != nil {
		log.Println("failed reading gemdrive_db.json")
		dbJson = []byte("{}")
	}

	var db *Database

	err = json.Unmarshal(dbJson, &db)
	if err != nil {
		log.Println(err)
		db = &Database{}
	}

	db.mut = &sync.Mutex{}

	db.persist()

	return db
}

func (db *Database) GetKey(token string) (*Key, error) {
	db.mut.Lock()
	defer db.mut.Unlock()

	key, exists := db.Keys[token]
	if !exists {
		return nil, errors.New("Does not exist")
	}

	return key, nil
}

func (db *Database) persist() {
	saveJson(db, "gemdrive_db.json")
}

func NewAuth(metaRoot string) *Auth {
	db := NewDatabase()
	return &Auth{metaRoot, db}
}

func (a *Auth) CanRead(token, pathStr string) bool {

	key, err := a.db.GetKey(token)
	if err != nil {
		return false
	}

	if !key.CanRead(pathStr) {
		return false
	}

	acl := a.GetAcl(pathStr)

	return acl.CanRead(key.Id)
}

func (a *Auth) CanWrite(token, pathStr string) bool {

	key, err := a.db.GetKey(token)
	if err != nil {
		return false
	}

	if !key.CanWrite(pathStr) {
		return false
	}

	acl := a.GetAcl(pathStr)

	return acl.CanWrite(key.Id)
}

func (a *Auth) GetAcl(pathStr string) Acl {

	parts := strings.Split(pathStr, "/")

	for i := len(parts) - 1; i > 0; i-- {
		p := strings.Join(parts[:i], "/")
		aclPath := path.Join(a.metaRoot, p, "gemdrive", "acl.json")

		acl, err := readAcl(aclPath)
		if err == nil {
			return acl
		}
	}

	return Acl{}
}

func readAcl(pathStr string) (Acl, error) {
	aclBytes, err := ioutil.ReadFile(pathStr)
	if err != nil {
		return nil, err
	}

	var acl Acl
	err = json.Unmarshal(aclBytes, &acl)
	if err != nil {
		return nil, err
	}

	return acl, nil
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
	return perm == "write" || permCanOwn(perm)
}

func permCanOwn(perm string) bool {
	return perm == "own"
}
