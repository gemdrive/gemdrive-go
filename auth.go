package gemdrive

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/smtp"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type Auth struct {
	cacheDir            string
	db                  *Database
	config              *Config
	pendingAuthRequests map[string]*AuthRequest
	mut                 *sync.Mutex
}

type AuthRequest struct {
	code    string
	keyring []*Key
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

// TODO: Replace with Key?
type AclEntry struct {
	IdType string `json:"idType"`
	Id     string `json:"id"`
	Perm   string `json:"perm"`
}

type Key struct {
	IdType string `json:"idType"`
	Id     string `json:"id"`
	Perm   string `json:"perm"`
	Path   string `json:"path"`
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
	Keys map[string][]*Key `json:"keys"`
	mut  *sync.Mutex
	path string
}

func NewDatabase(dir string) *Database {

	dbPath := path.Join(dir, "gemdrive_auth_db.json")
	dbJson, err := ioutil.ReadFile(dbPath)
	if err != nil {
		log.Println("failed reading gemdrive_auth_db.json")
		dbJson = []byte("")
	}

	var db *Database

	err = json.Unmarshal(dbJson, &db)
	if err != nil {
		db = &Database{
			Keys: make(map[string][]*Key),
		}
	}

	db.path = dbPath

	db.mut = &sync.Mutex{}

	db.persist()

	return db
}

func (db *Database) GetKeyring(token string) ([]*Key, error) {
	db.mut.Lock()
	defer db.mut.Unlock()

	key, exists := db.Keys[token]
	if !exists {
		return nil, errors.New("Does not exist")
	}

	return key, nil
}

func (db *Database) SetKeyring(token string, keyring []*Key) {
	db.mut.Lock()
	defer db.mut.Unlock()

	db.Keys[token] = keyring

	db.persist()
}

func (db *Database) persist() {
	saveJson(db, db.path)
}

func NewAuth(cacheDir string, config *Config) (*Auth, error) {

	rootAclPath := path.Join(cacheDir, "gemdrive", "acl.json")

	err := os.MkdirAll(path.Dir(rootAclPath), 0755)
	if err != nil {
		return nil, err
	}

	_, err = os.Stat(rootAclPath)
	if os.IsNotExist(err) {
		entry := &AclEntry{
			IdType: "email",
			Id:     config.AdminEmail,
			Perm:   "own",
		}
		var acl Acl = []*AclEntry{entry}

		err := saveJson(acl, rootAclPath)
		if err != nil {
			return nil, err
		}
	}

	db := NewDatabase(cacheDir)

	pendingAuthRequests := make(map[string]*AuthRequest)
	mut := &sync.Mutex{}

	return &Auth{cacheDir, db, config, pendingAuthRequests, mut}, nil
}

func (a *Auth) Authorize(key Key) (string, error) {

	requestId, err := genRandomKey()
	if err != nil {
		return "", err
	}

	code, err := genCode()
	if err != nil {
		return "", err
	}

	bodyTemplate := "From: %s <%s>\r\n" +
		"To: %s\r\n" +
		"Subject: Email Verification\r\n" +
		"\r\n" +
		"An application wants to access your data. Use the following code to complete authorization:\r\n" +
		"\r\n" +
		"%s\r\n"

	fromText := "GemDrive email verifier"
	fromEmail := a.config.Smtp.Sender
	email := key.Id
	emailBody := fmt.Sprintf(bodyTemplate, fromText, fromEmail, email, code)

	emailAuth := smtp.PlainAuth("", a.config.Smtp.Username, a.config.Smtp.Password, a.config.Smtp.Server)
	srv := fmt.Sprintf("%s:%d", a.config.Smtp.Server, a.config.Smtp.Port)
	msg := []byte(emailBody)
	err = smtp.SendMail(srv, emailAuth, fromEmail, []string{email}, msg)
	if err != nil {
		return "", err
	}

	a.mut.Lock()
	a.pendingAuthRequests[requestId] = &AuthRequest{
		code:    code,
		keyring: []*Key{&key},
	}
	a.mut.Unlock()

	// Requests expire after a certain time
	go func() {
		time.Sleep(60 * time.Second)
		a.mut.Lock()
		delete(a.pendingAuthRequests, requestId)
		a.mut.Unlock()
	}()

	return requestId, nil
}

func (a *Auth) CompleteAuth(requestId, code string) (string, error) {

	a.mut.Lock()
	req, exists := a.pendingAuthRequests[requestId]
	delete(a.pendingAuthRequests, requestId)
	a.mut.Unlock()

	if exists && req.code == code {
		token, err := genRandomKey()
		if err != nil {
			return "", err
		}
		a.db.SetKeyring(token, req.keyring)
		return token, nil
	}

	return "", nil
}

func (a *Auth) CanRead(token, pathStr string) bool {

	acl := a.GetAcl(pathStr)

	keyring, err := a.db.GetKeyring(token)
	if err != nil {
		return false
	}

	for _, key := range keyring {
		if key.CanRead(pathStr) && acl.CanRead(key.Id) {
			return true
		}
	}

	return false
}

func (a *Auth) CanWrite(token, pathStr string) bool {

	acl := a.GetAcl(pathStr)

	keyring, err := a.db.GetKeyring(token)
	if err != nil {
		return false
	}

	for _, key := range keyring {
		if key.CanWrite(pathStr) && acl.CanWrite(key.Id) {
			return true
		}
	}

	return false
}

func (a *Auth) GetAcl(pathStr string) Acl {

	parts := strings.Split(pathStr, "/")

	for i := len(parts) - 1; i > 0; i-- {
		p := strings.Join(parts[:i], "/")
		aclPath := path.Join(a.cacheDir, p, "gemdrive", "acl.json")

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

func genCode() (string, error) {
	const chars string = "0123456789abcdefghijkmnpqrstuvwxyz"
	id := ""
	for i := 0; i < 4; i++ {
		randIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		id += string(chars[randIndex.Int64()])
	}
	return id, nil
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
