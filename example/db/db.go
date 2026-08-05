package db

import "fmt"

type Pinger interface {
	Ping()
}

type DB struct {
	Name string
	DSN  string
}
type RedisDbIndex int

var Index RedisDbIndex = RedisDbIndex(0)

func (d *DB) Ping() {
	fmt.Printf("[%s] Ping: %s\n", d.Name, d.DSN)
}

type RedisClient struct {
	Name    string
	Address string
}

func (r *RedisClient) Ping(index RedisDbIndex) {
	fmt.Printf("[%s] Redis Ping: %s,db=%v\n", r.Name, r.Address, Index)
}
