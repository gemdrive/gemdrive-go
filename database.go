package gemdrive

import (
	//"fmt"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"path/filepath"
	"sync"
)

type GemDriveDatabase struct {
	Keys               map[string]*KeyData `json:"keys"`
	WaygateAccessToken string              `json:"waygate_access_token"`
	waygateState       string
	dbPath             string
	mutex              *sync.Mutex
}

func NewGemDriveDatabase(dir string) (*GemDriveDatabase, error) {

	dbPath := filepath.Join(dir, "gemdrive_db.json")

	db := &GemDriveDatabase{
		Keys:   make(map[string]*KeyData),
		dbPath: dbPath,
		mutex:  &sync.Mutex{},
	}

	dbJson, err := ioutil.ReadFile(dbPath)
	if err == nil {
		err = json.Unmarshal(dbJson, &db)
		if err != nil {
			log.Println(err)
		}
	}

	_, err = db.GetMasterKey()
	if err != nil {
		key, err := genRandomKey()
		if err != nil {
			log.Println(err)
		}

		masterKeyData := &KeyData{
			Parent: "",
			Privileges: map[string]string{
				"/": "write",
			},
		}
		db.AddKeyData(key, masterKeyData)
		db.Persist()
	}

	return db, nil
}

func (db GemDriveDatabase) Persist() error {
	saveJson(db, db.dbPath)
	return nil
}

func (db *GemDriveDatabase) AddKeyData(key string, keyData *KeyData) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, exists := db.Keys[key]

	if exists {
		return errors.New("Key exists")
	}

	db.Keys[key] = keyData

	db.Persist()

	return nil
}

func (db *GemDriveDatabase) DeleteKeyData(key string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, exists := db.Keys[key]

	if !exists {
		return errors.New("No such key")
	}

	delete(db.Keys, key)

	db.Persist()

	return nil
}

func (db *GemDriveDatabase) GetMasterKey() (string, error) {

	db.mutex.Lock()
	defer db.mutex.Unlock()

	for key, keyData := range db.Keys {
		if keyData.Parent == "" {
			return key, nil
		}
	}

	return "", errors.New("No master key")
}

func (db *GemDriveDatabase) GetKeyData(key string) (*KeyData, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	keyData, exists := db.Keys[key]

	if !exists {
		return nil, errors.New("Invalid key")
	}

	return keyData, nil
}

// Waygate methods
func (db *GemDriveDatabase) GetState() string {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.waygateState
}
func (db *GemDriveDatabase) SetState(state string) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.waygateState = state

	db.Persist()
}
func (db *GemDriveDatabase) GetAccessToken() (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.WaygateAccessToken == "" {
		return "", errors.New("No access token")
	}

	return db.WaygateAccessToken, nil
}
func (db *GemDriveDatabase) SetAccessToken(token string) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.WaygateAccessToken = token

	db.Persist()
}
