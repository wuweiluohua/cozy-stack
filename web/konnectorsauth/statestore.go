package konnectorsauth

import (
	"encoding/json"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/go-redis/redis"
)

const stateTTL = 15 * time.Minute

type stateHolder struct {
	InstanceDomain string
	AccountType    string
	ClientState    string
	ExpiresAt      int64
}

type stateStorage interface {
	Add(*stateHolder) (string, error)
	Find(ref string) *stateHolder
}

type memStateStorage map[string]*stateHolder

func (store memStateStorage) Add(state *stateHolder) (string, error) {
	state.ExpiresAt = time.Now().UTC().Add(stateTTL).Unix()
	ref := utils.RandomString(16)
	store[ref] = state
	return ref, nil
}

func (store memStateStorage) Find(ref string) *stateHolder {

	state, ok := store[ref]
	if !ok {
		return nil
	}

	if state.ExpiresAt < time.Now().UTC().Unix() {
		delete(store, ref)
		return nil
	}

	return state
}

type subRedisInterface interface {
	Get(key string) *redis.StringCmd
	Set(key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type redisStateStorage struct {
	cl subRedisInterface
}

func (store *redisStateStorage) Add(s *stateHolder) (string, error) {
	ref := utils.RandomString(16)
	bb, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return ref, store.cl.Set(ref, bb, stateTTL).Err()
}

func (store *redisStateStorage) Find(ref string) *stateHolder {
	bb, err := store.cl.Get(ref).Bytes()
	if err != nil {
		return nil
	}
	var s stateHolder
	err = json.Unmarshal(bb, &s)
	if err != nil {
		log.Errorf("[redis-oauth-state] bad state in redis %s", string(bb))
		return nil
	}
	return &s
}

var globalStorage stateStorage
var globalStorageMutex sync.Mutex

func makeStorage() stateStorage {
	opts := config.GetConfig().KonnectorsOauthStateStorage.Options()
	if opts == nil {
		return &memStateStorage{}
	}

	client := redis.NewClient(opts)
	return &redisStateStorage{
		cl: client,
	}
}

func getStorage() stateStorage {
	globalStorageMutex.Lock()
	defer globalStorageMutex.Unlock()
	if globalStorage == nil {
		globalStorage = makeStorage()
	}
	return globalStorage
}
