package dao

import (
	"github.com/mcdexio/mai3-broker/common/model"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
)

type KVStoreDAO interface {
	Get(key, category string) (*model.KVStore, error)
	Put(value *model.KVStore) error
	Del(key string) error
	List(category ...string) ([]*model.KVStore, error)
}

type kvstoreDAO struct {
	db *gorm.DB
}

func NewKVStoreDAO(db *gorm.DB) KVStoreDAO {
	return &kvstoreDAO{db: db}
}

func (n *kvstoreDAO) Get(key, category string) (*model.KVStore, error) {
	kv := new(model.KVStore)
	if err := n.db.Where("key = ? and category = ?", key, category).First(kv).Error; err != nil {
		return nil, errors.Wrap(err, "get kvstore entry failed")
	}
	return kv, nil
}

func (n *kvstoreDAO) Put(value *model.KVStore) error {
	value.UpdateTime = time.Now()
	return n.db.Save(value).Error
}

func (n *kvstoreDAO) Del(key string) error {
	return n.db.Delete(&model.KVStore{}, "key = ?", key).Error
}

func (n *kvstoreDAO) List(category ...string) ([]*model.KVStore, error) {
	var kvs []*model.KVStore
	if err := n.db.Where("category in (?)", category).Find(&kvs).Error; err != nil {
		return nil, errors.Wrap(err, "list kvs failed")
	}
	return kvs, nil
}
