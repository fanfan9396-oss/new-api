package middleware

import (
	"context"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

const modelConcurrencyNamespace = "modelConcurrency:v1"

const modelConcurrencyTakeScript = `
local userCount = redis.call('INCR', KEYS[1])
local globalCount = redis.call('INCR', KEYS[2])
if userCount > tonumber(ARGV[1]) or globalCount > tonumber(ARGV[2]) then
  redis.call('DECR', KEYS[1])
  redis.call('DECR', KEYS[2])
  return 0
end
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('EXPIRE', KEYS[2], ARGV[3])
return 1
`

const modelConcurrencyReleaseScript = `
local userCount = redis.call('DECR', KEYS[1])
local globalCount = redis.call('DECR', KEYS[2])
if userCount <= 0 then redis.call('DEL', KEYS[1]) end
if globalCount <= 0 then redis.call('DEL', KEYS[2]) end
return 1
`

var modelConcurrencyMemory = struct {
	sync.Mutex
	users  map[int]int
	global int
}{users: make(map[int]int)}

func resetModelConcurrencyForTest() {
	modelConcurrencyMemory.Lock()
	defer modelConcurrencyMemory.Unlock()
	modelConcurrencyMemory.users = make(map[int]int)
	modelConcurrencyMemory.global = 0
}

func modelConcurrencyKeys(userID int) []string {
	return []string{
		fmt.Sprintf("%s:user:%d", modelConcurrencyNamespace, userID),
		fmt.Sprintf("%s:global", modelConcurrencyNamespace),
	}
}

func admitModelConcurrency(ctx context.Context, userID, perUser, global int) (func(), bool) {
	if userID <= 0 || perUser <= 0 || global <= 0 {
		return nil, false
	}
	keys := modelConcurrencyKeys(userID)
	if common.RedisEnabled && common.RDB != nil {
		result, err := common.RDB.Eval(ctx, modelConcurrencyTakeScript, keys, perUser, global, 120).Int()
		if err != nil || result != 1 {
			return nil, false
		}
		return func() {
			_, _ = common.RDB.Eval(context.Background(), modelConcurrencyReleaseScript, keys).Result()
		}, true
	}

	modelConcurrencyMemory.Lock()
	defer modelConcurrencyMemory.Unlock()
	if modelConcurrencyMemory.users[userID] >= perUser || modelConcurrencyMemory.global >= global {
		return nil, false
	}
	modelConcurrencyMemory.users[userID]++
	modelConcurrencyMemory.global++
	var once sync.Once
	return func() {
		once.Do(func() {
			modelConcurrencyMemory.Lock()
			defer modelConcurrencyMemory.Unlock()
			modelConcurrencyMemory.users[userID]--
			if modelConcurrencyMemory.users[userID] <= 0 {
				delete(modelConcurrencyMemory.users, userID)
			}
			modelConcurrencyMemory.global--
		})
	}, true
}

func modelConcurrencyAdmission(c *gin.Context) (func(), bool) {
	return admitModelConcurrency(c.Request.Context(), c.GetInt("id"), setting.ModelRequestConcurrencyPerUser, setting.ModelRequestConcurrencyGlobal)
}
