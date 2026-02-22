package secrets

import (
	"encoding/json"
	"time"

	"github.com/99designs/keyring"
)

const serviceName = "krocli"

type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
	TokenType    string    `json:"token_type"`
}

func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: serviceName,
	})
}

func StoreToken(key string, t *TokenData) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return kr.Set(keyring.Item{
		Key:  key,
		Data: data,
	})
}

func LoadToken(key string) (*TokenData, error) {
	kr, err := openKeyring()
	if err != nil {
		return nil, err
	}
	item, err := kr.Get(key)
	if err != nil {
		return nil, err
	}
	var t TokenData
	if err := json.Unmarshal(item.Data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func DeleteToken(key string) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	return kr.Remove(key)
}
