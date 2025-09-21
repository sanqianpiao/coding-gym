package bucket

import "github.com/go-redis/redis/v8"

// Lua script for atomic token consumption
const takeTokensScript = `
local key = KEYS[1]
local requested_tokens = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local refill_rate = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local now = redis.call('TIME')
local current_time = tonumber(now[1]) + tonumber(now[2]) / 1000000

-- Get current bucket state
local bucket_data = redis.call('HMGET', key, 'tokens', 'last_refill')
local current_tokens = tonumber(bucket_data[1]) or capacity
local last_refill = tonumber(bucket_data[2]) or current_time

-- Calculate tokens to add based on time elapsed
local time_elapsed = math.max(0, current_time - last_refill)
local tokens_to_add = time_elapsed * refill_rate
local new_tokens = math.min(capacity, current_tokens + tokens_to_add)

-- Check if we have enough tokens
local allowed = 0
local retry_after = 0

if new_tokens >= requested_tokens then
    -- Consume tokens
    new_tokens = new_tokens - requested_tokens
    allowed = 1
else
    -- Calculate retry after time
    local tokens_needed = requested_tokens - new_tokens
    retry_after = tokens_needed / refill_rate
end

-- Update bucket state
redis.call('HMSET', key, 
    'tokens', new_tokens,
    'last_refill', current_time,
    'capacity', capacity,
    'refill_rate', refill_rate
)

-- Set TTL
redis.call('EXPIRE', key, ttl)

return {allowed, new_tokens, retry_after}
`

// Lua script for getting bucket state
const getBucketStateScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local now = redis.call('TIME')
local current_time = tonumber(now[1]) + tonumber(now[2]) / 1000000

-- Get current bucket state
local bucket_data = redis.call('HMGET', key, 'tokens', 'last_refill')
local current_tokens = tonumber(bucket_data[1]) or capacity
local last_refill = tonumber(bucket_data[2]) or current_time

-- Calculate tokens to add based on time elapsed
local time_elapsed = math.max(0, current_time - last_refill)
local tokens_to_add = time_elapsed * refill_rate
local new_tokens = math.min(capacity, current_tokens + tokens_to_add)

-- Update bucket state if tokens were added
if tokens_to_add > 0 then
    redis.call('HMSET', key, 
        'tokens', new_tokens,
        'last_refill', current_time,
        'capacity', capacity,
        'refill_rate', refill_rate
    )
    redis.call('EXPIRE', key, ttl)
end

-- Get TTL
local bucket_ttl = redis.call('TTL', key)
if bucket_ttl == -1 then
    bucket_ttl = ttl
    redis.call('EXPIRE', key, ttl)
end

return {new_tokens, current_time, bucket_ttl}
`

// Lua script for resetting a bucket
const resetBucketScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local now = redis.call('TIME')
local current_time = tonumber(now[1]) + tonumber(now[2]) / 1000000

-- Reset bucket to full capacity
redis.call('HMSET', key, 
    'tokens', capacity,
    'last_refill', current_time,
    'capacity', capacity,
    'refill_rate', refill_rate
)

redis.call('EXPIRE', key, ttl)

return {capacity, current_time}
`

// initLuaScripts initializes all Lua scripts
func (tb *RedisTokenBucket) initLuaScripts() {
	tb.luaScripts["take_tokens"] = redis.NewScript(takeTokensScript)
	tb.luaScripts["get_state"] = redis.NewScript(getBucketStateScript)
	tb.luaScripts["reset_bucket"] = redis.NewScript(resetBucketScript)
}
