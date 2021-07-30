package dao

import (
	"fmt"
	"time"

	"github.com/mcdexio/mai3-broker/common/model"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
)

type LaunchTransactionDAO interface {
	FirstTx(status ...model.LaunchTransactionStatus) (*model.LaunchTransaction, error)
	FirstTxByUser(addr string, status ...model.LaunchTransactionStatus) (*model.LaunchTransaction, error)
	GetTxsByUser(addr string, blockNumber *uint64, status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error)
	GetTxByID(txID string) (*model.LaunchTransaction, error)
	GetTxByHash(txHash string) (*model.LaunchTransaction, error)
	GetTxsByNonce(user string, nonce *uint64, status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error)
	GetTxsByBlock(begin *uint64, end *uint64, status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error)
	GetUsersWithStatus(status ...model.LaunchTransactionStatus) ([]string, error)

	CreateTx(tx *model.LaunchTransaction) error
	UpdateTx(tx *model.LaunchTransaction) error
}

func NewLaunchTransactionDAO(db *gorm.DB) LaunchTransactionDAO {
	return &launchTransactionDAO{db: db}
}

type launchTransactionDAO struct {
	db *gorm.DB
}

func (t *launchTransactionDAO) FirstTx(status ...model.LaunchTransactionStatus) (*model.LaunchTransaction, error) {
	tx := new(model.LaunchTransaction)
	err := t.statusFilter(status).
		Order("commit_time", true).
		First(tx).Error
	if err != nil {
		return nil, fmt.Errorf("get pending transaction failed: %w", err)
	}
	return tx, nil
}

func (t *launchTransactionDAO) FirstTxByUser(
	addr string,
	status ...model.LaunchTransactionStatus) (*model.LaunchTransaction, error) {

	tx := new(model.LaunchTransaction)
	err := t.statusFilter(status).
		Where("from_address = ?", addr).
		Order("nonce").
		Find(tx).Error
	if err != nil {
		return nil, fmt.Errorf("check pending transactions failed: %w", err)
	}
	return tx, nil
}

func (t *launchTransactionDAO) GetTxsByUser(
	addr string,
	blockNumber *uint64,
	status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error) {

	var txs []*model.LaunchTransaction
	if blockNumber != nil {
		t.db = t.db.Where("block_number >= ?", *blockNumber)
	}
	err := t.statusFilter(status).
		Where("from_address = ?", addr).
		Order("nonce").
		Find(&txs).Error
	if err != nil {
		return nil, fmt.Errorf("check pending transactions failed: %w", err)
	}
	return txs, nil
}

func (t *launchTransactionDAO) GetTxByID(txID string) (*model.LaunchTransaction, error) {
	txs, err := t.findAll(&txID, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "find all transaction failed")
	}
	if len(txs) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	for _, tx := range txs {
		if tx.Status == model.TxSuccess {
			return tx, nil
		}
	}
	return txs[0], nil
}

func (t *launchTransactionDAO) GetTxByHash(txHash string) (*model.LaunchTransaction, error) {
	return t.find(nil, &txHash, nil)
}

func (t *launchTransactionDAO) GetTxsByNonce(addr string, nonce *uint64, status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error) {
	if nonce == nil {
		return nil, fmt.Errorf("Missing nonce")
	}
	var txs []*model.LaunchTransaction
	err := t.statusFilter(status).
		Where("from_address = ? AND nonce = ?", addr, *nonce).
		Find(&txs).Error
	if err != nil {
		return nil, errors.Wrap(err, "fail to find transaction by nonce")
	}
	return txs, nil
}

func (t *launchTransactionDAO) GetTxsByBlock(begin *uint64, end *uint64, status ...model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error) {

	var txs []*model.LaunchTransaction
	db := t.statusFilter(status)
	if begin != nil {
		db = db.Where("block_number >= ?", *begin)
	}
	if end != nil {
		db = db.Where("block_number <= ?", *end)
	}
	if err := db.Find(&txs).Error; err != nil {
		return nil, errors.Wrap(err, "fail to find transaction by block")
	}
	return txs, nil
}

func (t *launchTransactionDAO) GetUsersWithStatus(status ...model.LaunchTransactionStatus) ([]string, error) {

	var (
		txs   []*model.LaunchTransaction
		users []string
	)
	err := t.statusFilter(status).Select("DISTINCT(from_address)").Find(&txs).Error
	if err != nil {
		return nil, fmt.Errorf("fail to get pending users: %w", err)
	}
	for _, tx := range txs {
		users = append(users, tx.FromAddress)
	}
	return users, nil
}

// New insert a new record to database
func (t *launchTransactionDAO) CreateTx(tx *model.LaunchTransaction) error {
	tx.UpdateTime = time.Now()
	return t.db.Create(tx).Error
}

func (t *launchTransactionDAO) UpdateTx(tx *model.LaunchTransaction) error {
	tx.UpdateTime = time.Now()
	return t.db.Save(tx).Error
}

func (t *launchTransactionDAO) find(txID *string, txHash *string, status []model.LaunchTransactionStatus) (*model.LaunchTransaction, error) {

	if txID == nil && txHash == nil {
		return nil, fmt.Errorf("txID, txHash cannot be nil")
	}
	db := t.statusFilter(status)
	if txID != nil {
		db = db.Where("tx_id = ?", txID)
	}
	if txHash != nil {
		db = db.Where("transaction_hash = ?", txHash)
	}
	tx := new(model.LaunchTransaction)
	if err := db.First(tx).Error; err != nil {
		return nil, errors.Wrap(err, "get transaction failed")
	}
	return tx, nil
}

func (t *launchTransactionDAO) findAll(txID *string, txHash *string, status []model.LaunchTransactionStatus) ([]*model.LaunchTransaction, error) {

	if txID == nil && txHash == nil {
		return nil, fmt.Errorf("txID, txHash cannot be nil")
	}
	db := t.statusFilter(status)
	if txID != nil {
		db = db.Where("tx_id = ?", txID)
	}
	if txHash != nil {
		db = db.Where("transaction_hash = ?", txHash)
	}
	var txs []*model.LaunchTransaction
	if err := db.Order("commit_time", true).Find(&txs).Error; err != nil {
		return nil, errors.Wrap(err, "get transaction failed")
	}
	return txs, nil
}

func (t *launchTransactionDAO) statusFilter(status []model.LaunchTransactionStatus) *gorm.DB {
	if len(status) > 0 {
		return t.db.Where("status in (?)", status)
	}
	return t.db
}
