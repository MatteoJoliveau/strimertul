package database

import (
	"errors"
	"fmt"

	jsoniter "github.com/json-iterator/go"
	kv "github.com/strimertul/kilovolt/v9"
	"go.uber.org/zap"
)

type CancelFunc func()

var json = jsoniter.ConfigFastest

var (
	// ErrUnknown is returned when a response is received that doesn't match any expected outcome.
	ErrUnknown = errors.New("unknown error")

	// ErrEmptyKey is when a key is requested as JSON object but is an empty string (or unset)
	ErrEmptyKey = errors.New("empty key")
)

type LocalDBClient struct {
	client *kv.LocalClient
	hub    *kv.Hub
	logger *zap.Logger
}

type KvPair struct {
	Key  string
	Data string
}

func NewLocalClient(hub *kv.Hub, logger *zap.Logger) (*LocalDBClient, error) {
	// Create local client
	localClient := kv.NewLocalClient(kv.ClientOptions{}, logger)

	// Run client and add it to the hub
	go localClient.Run()
	hub.AddClient(localClient)
	localClient.Wait()

	// Bypass authentication
	err := hub.SetAuthenticated(localClient.UID(), true)
	if err != nil {
		return nil, err
	}

	return &LocalDBClient{
		client: localClient,
		hub:    hub,
		logger: logger,
	}, nil
}

func (mod *LocalDBClient) Hub() *kv.Hub {
	return mod.hub
}

func (mod *LocalDBClient) Close() error {
	mod.hub.RemoveClient(mod.client)
	return nil
}

func (mod *LocalDBClient) GetKey(key string) (string, error) {
	res, err := mod.makeRequest(kv.CmdReadKey, map[string]interface{}{"key": key})
	if err != nil {
		return "", err
	}
	return res.Data.(string), nil
}

func (mod *LocalDBClient) PutKey(key string, data string) error {
	_, err := mod.makeRequest(kv.CmdWriteKey, map[string]interface{}{"key": key, "data": data})
	return err
}

func (mod *LocalDBClient) SubscribePrefix(fn kv.SubscriptionCallback, prefixes ...string) (err error, cancelFn CancelFunc) {
	var ids []int64
	for _, prefix := range prefixes {
		_, err = mod.makeRequest(kv.CmdSubscribePrefix, map[string]interface{}{"prefix": prefix})
		if err != nil {
			return err, nil
		}
		ids = append(ids, mod.client.SetPrefixSubCallback(prefix, fn))
	}
	return nil, func() {
		for _, id := range ids {
			mod.client.UnsetCallback(id)
		}
	}
}

func (mod *LocalDBClient) SubscribeKey(key string, fn func(string)) (err error, cancelFn CancelFunc) {
	_, err = mod.makeRequest(kv.CmdSubscribePrefix, map[string]interface{}{"prefix": key})
	if err != nil {
		return err, nil
	}
	id := mod.client.SetPrefixSubCallback(key, func(changedKey string, value string) {
		if key != changedKey {
			return
		}
		fn(value)
	})
	return nil, func() {
		mod.client.UnsetCallback(id)
	}
}

func (mod *LocalDBClient) GetJSON(key string, dst interface{}) error {
	res, err := mod.GetKey(key)
	if err != nil {
		return err
	}
	if res == "" {
		return ErrEmptyKey
	}
	return json.Unmarshal([]byte(res), dst)
}

func (mod *LocalDBClient) GetAll(prefix string) (map[string]string, error) {
	res, err := mod.makeRequest(kv.CmdReadPrefix, map[string]interface{}{"prefix": prefix})
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	for key, value := range res.Data.(map[string]interface{}) {
		out[key] = value.(string)
	}
	return out, nil
}

func (mod *LocalDBClient) PutJSON(key string, data interface{}) error {
	byt, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return mod.PutKey(key, string(byt))
}

func (mod *LocalDBClient) PutJSONBulk(kvs map[string]interface{}) error {
	encoded := make(map[string]interface{})
	for k, v := range kvs {
		byt, err := json.Marshal(v)
		if err != nil {
			return err
		}
		encoded[k] = string(byt)
	}
	_, chn := mod.client.MakeRequest(kv.CmdWriteBulk, encoded)
	_, err := getResponse(<-chn)
	return err
}

func (mod *LocalDBClient) RemoveKey(key string) error {
	// TODO
	return mod.PutKey(key, "")
}

func (mod *LocalDBClient) makeRequest(cmd string, data map[string]interface{}) (kv.Response, error) {
	req, chn := mod.client.MakeRequest(cmd, data)
	mod.hub.SendMessage(req)
	return getResponse(<-chn)
}

func getResponse(response interface{}) (kv.Response, error) {
	switch c := response.(type) {
	case kv.Response:
		return c, nil
	case kv.Error:
		return kv.Response{}, &KvError{c}
	}
	return kv.Response{}, ErrUnknown
}

type KvError struct {
	ErrorData kv.Error
}

func (kv *KvError) Error() string {
	return fmt.Sprintf("%s: %s", kv.ErrorData.Error, kv.ErrorData.Details)
}
