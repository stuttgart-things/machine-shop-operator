package main

import (
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/stuttgart-things/redisqueue"
)

func main() {

	p, err := redisqueue.NewProducerWithOptions(&redisqueue.ProducerOptions{
		MaxLen:               10000,
		ApproximateMaxLength: true,
		RedisClient: redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDIS_SERVER") + ":" + os.Getenv("REDIS_PORT"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		}),
	})

	if err != nil {
		panic(err)
	}

	err2 := p.Enqueue(&redisqueue.Message{
		Stream: "q9:1",
		Values: map[string]interface{}{
			"name": "ankit",
		},
	})

	if err2 != nil {
		panic(err)
	}

}
