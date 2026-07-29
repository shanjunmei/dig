package db

import "fmt"

type DB struct {
	Name string
	DSN  string
}

func (d *DB) Ping() {
	fmt.Printf("[%s] Ping: %s\n", d.Name, d.DSN)
}

type RedisClient struct {
	Name    string
	Address string
}

func (r *RedisClient) Ping() {
	fmt.Printf("[%s] Redis Ping: %s\n", r.Name, r.Address)
}
