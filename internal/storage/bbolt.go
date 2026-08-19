package storage

import (
	"time"

	"go.etcd.io/bbolt"
)

type DB struct {
	db *bbolt.DB
}

func Open(path string) (*DB, error) {
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *DB) Update(fn func(tx *bbolt.Tx) error) error {
	return d.db.Update(fn)
}

func (d *DB) View(fn func(tx *bbolt.Tx) error) error {
	return d.db.View(fn)
}

func (d *DB) Bucket(name string) *Bucket {
	return &Bucket{db: d, name: []byte(name)}
}

type Bucket struct {
	db   *DB
	name []byte
}

func (b *Bucket) Name() []byte { return b.name }

func (b *Bucket) Put(key, value []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(b.name)
		if err != nil {
			return err
		}
		return bucket.Put(key, value)
	})
}

func (b *Bucket) Get(key []byte) ([]byte, error) {
	var val []byte
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.name)
		if bucket == nil {
			return nil
		}
		v := bucket.Get(key)
		if v != nil {
			val = make([]byte, len(v))
			copy(val, v)
		}
		return nil
	})
	return val, err
}

func (b *Bucket) Delete(key []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.name)
		if bucket == nil {
			return nil
		}
		return bucket.Delete(key)
	})
}

func (b *Bucket) ForEach(fn func(k, v []byte) error) error {
	return b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.name)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(fn)
	})
}

func (b *Bucket) CreateIfNotExists() error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(b.name)
		return err
	})
}