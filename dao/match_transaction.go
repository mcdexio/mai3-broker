package dao

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/mcdexio/mai3-broker/common/model"
)

type MatchTransactionDAO interface {
	CreateMatchTransaction(*model.MatchTransaction) error
	QueryMatchTransaction(poolAddress string, perpIndex int64, status []model.TransactionStatus) ([]*model.MatchTransaction, error)
	QueryUnconfirmedTransactions() ([]*model.MatchTransaction, error)
	QueryUnconfirmedTransactionsByContract(poolAddress string, perpIndex int64) (transactions []*model.MatchTransaction, err error)
	GetMatchTransaction(ID string) (*model.MatchTransaction, error)
	UpdateMatchTransaction(transaction *model.MatchTransaction) error
}

type matchTransactionDAO struct {
	db *gorm.DB
}

func (t *matchTransactionDAO) CreateMatchTransaction(transaction *model.MatchTransaction) error {
	jsonData, err := json.Marshal(transaction.MatchResult)
	if err != nil {
		return fmt.Errorf("CreateMatchTransaction:%w", err)
	}
	transaction.MatchJson = string(jsonData)
	transaction.CreatedAt = time.Now().UTC()
	if err := t.db.Create(transaction).Error; err != nil {
		return fmt.Errorf("CreateMatchTransaction:%w", err)
	}
	return nil
}

func (t *matchTransactionDAO) GetMatchTransaction(ID string) (*model.MatchTransaction, error) {
	var transaction model.MatchTransaction

	db := t.db
	if err := db.Where("id = ?", ID).First(&transaction).Error; err != nil {
		return nil, fmt.Errorf("GetMatchTransaction:%w", err)
	}

	if err := json.Unmarshal([]byte(transaction.MatchJson), &transaction.MatchResult); err != nil {
		return nil, fmt.Errorf("GetMatchTransaction:%w", err)
	}

	return &transaction, nil
}

func (t *matchTransactionDAO) QueryMatchTransaction(poolAddress string, perpIndex int64, status []model.TransactionStatus) (transactions []*model.MatchTransaction, err error) {
	db := t.db
	if poolAddress != "" {
		db = db.Where("liquidity_pool_address = ? AND perpetual_index = ?", poolAddress, perpIndex)
	}
	if len(status) != 0 {
		db = db.Where("status in (?)", status)
	}
	if err = db.Order("created_at").Find(&transactions).Error; err != nil {
		err = fmt.Errorf("QueryMatchTransaction:%w", err)
		return
	}

	for _, transaction := range transactions {
		if err := json.Unmarshal([]byte(transaction.MatchJson), &transaction.MatchResult); err != nil {
			return nil, fmt.Errorf("QueryMatchTransaction:%w", err)
		}
	}
	return
}

func (t *matchTransactionDAO) UpdateMatchTransaction(transaction *model.MatchTransaction) error {
	jsonData, err := json.Marshal(transaction.MatchResult)
	if err != nil {
		return fmt.Errorf("CreateMatchTransaction:%w", err)
	}
	transaction.MatchJson = string(jsonData)
	if err = t.db.Save(transaction).Error; err != nil {
		return fmt.Errorf("UpdateMatchTransaction:%w", err)
	}
	return nil
}

func (t *matchTransactionDAO) QueryUnconfirmedTransactions() (transactions []*model.MatchTransaction, err error) {
	if err = t.db.Where("block_confirmed = ?", false).Find(&transactions).Error; err != nil {
		err = fmt.Errorf("QueryUnconfirmedTransactions:%w", err)
		return
	}
	return
}

func (t *matchTransactionDAO) QueryUnconfirmedTransactionsByContract(poolAddress string, perpIndex int64) (transactions []*model.MatchTransaction, err error) {
	if err = t.db.Where("liquidity_pool_address = ? AND perpetual_index = ?", poolAddress, perpIndex).Where("block_confirmed = ?", false).Find(&transactions).Error; err != nil {
		err = fmt.Errorf("QueryUnconfirmedTransactionsByContract:%w", err)
		return
	}
	return
}
