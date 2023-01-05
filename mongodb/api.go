package mongodb

import (
	"errors"
	"time"

	"github.com/anyswap/CrossChain-Bridge/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/weijun-sh/gethscan/params"
)

const (
	retryDBCount    = 3
	retryDBInterval = 1 * time.Second
)

// TryDoTimes try do again if meet error
func TryDoTimes(name string, f func() error) (err error) {
	for i := 0; i < retryDBCount; i++ {
		err = f()
		if err == nil || mgo.IsDup(err) {
			return nil
		}
		time.Sleep(retryDBInterval)
	}
	log.Warn("[mongodb] TryDoTimes", "name", name, "times", retryDBCount, "err", err)
	return err
}

// --------------- add ---------------------------------

// AddSwap and swap
func AddSwap(ms *MgoSwap, overwrite bool) (err error) {
	if overwrite {
		_, err = collectionSwap.UpsertId(ms.Id, ms)
		return err
	} else {
		err = collectionSwap.Insert(ms)
	}
	if err == nil {
		log.Info("[mongodb] AddSwap success", "swap", ms)
	} else {
		log.Warn("[mongodb] AddSwap failed", "swap", ms, "err", err)
	}
	return err
}

// AddSwapPending add pending
func AddSwapPending(ms *MgoSwap, overwrite bool) (err error) {
	if overwrite {
		_, err = collectionSwapPending.UpsertId(ms.Id, ms)
	} else {
		err = collectionSwapPending.Insert(ms)
	}
	if err == nil {
		log.Info("[mongodb] AddSwapPending success", "pending", ms)
	} else {
		log.Warn("[mongodb] AddSwapPending failed", "pending", ms, "err", err)
	}
	return err
}

// AddSwapDeleted add deleted
func AddSwapDeleted(ms *MgoSwap, overwrite bool) (err error) {
	if overwrite {
		_, err = collectionSwapDeleted.UpsertId(ms.Id, ms)
	} else {
		err = collectionSwapDeleted.Insert(ms)
	}
	if err == nil {
		log.Info("[mongodb] AddSwapDeleted success", "delete", ms)
	} else {
		log.Warn("[mongodb] AddSwapDeleted failed", "delete", ms, "err", err)
	}
	return err
}

// RemoveSwapPending add remove pending
func RemoveSwapPending(ms *MgoSwap) (err error) {
	err = collectionSwapPending.Remove(ms)
	if err == nil {
		log.Info("[mongodb] RemoveSwapPending success", "pending", ms)
	} else {
		log.Warn("[mongodb] RemoveSwapPending failed", "pending", ms, "err", err)
	}
	return err
}

// --------------- find ---------------------------------
// FindswapPending find by swap
func FindswapPending(swap string) (*MgoSwap, error) {
	var res MgoSwap
	//err := collectionSwapPending.Find(bson.M{"rpcMethod":swap}).One(&res)
	err := collectionSwapPending.Find(nil).One(&res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// FindAllSwapPending find SwapPending
func findAllSwapPending(chain string) *mgo.Iter {
	iter := collectionSwapPending.Find(bson.M{"chain": chain}).Iter()
	return iter
}

// FindAllTokenAccounts find accounts
func FindAllSwapPending(chain string, offset, limit int) ([]*MgoSwap, error) {
	result := make([]*MgoSwap, 0, limit)
	q := collectionSwapPending.Find(bson.M{"chain": chain}).Skip(offset).Limit(limit)
	err := q.All(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func UpdateSwapPending(swap *MgoSwap) {
	RemoveSwapPending(swap)

	swap.Timestamp = uint64(time.Now().Unix())
	AddSwap(swap, false)
}

func FindSyncedBlockNumber(chain string) (uint64, error) {
	var res SyncedBlock
	err := collectionSyncedBlock.Find(bson.M{"chain": chain}).One(&res)
	if err != nil {
		return 0, errors.New("mgo find failed")
	}
	return res.BlockNumber, nil
}

func InsertSyncedBlockNumber(chain string, number uint64) error {
	sb := SyncedBlock{}
	sb.Id = chain
	sb.Chain = chain
	sb.BlockNumber = number
	return collectionSyncedBlock.Insert(&sb)
}

func InitSyncedBlockNumber(chain string, number uint64) error {
	_, err := FindSyncedBlockNumber(chain)
	if err == nil {
		return UpdateSyncedBlockNumber(chain, number)
	}
	sb := SyncedBlock{}
	sb.Id = chain
	sb.Chain = chain
	sb.BlockNumber = number
	return collectionSyncedBlock.Insert(&sb)
}

func UpdateSyncedBlockNumber(chain string, number uint64) error {
	selector := bson.M{"chain": chain}
	data := bson.M{"$set": bson.M{"blocknumber": number}}
	err := collectionSyncedBlock.Update(selector, data)
	return err
}

// query p2sh

type p2shAddressConfig struct {
	Id         string `bson:"_id"`
	Bind       string `bson:"bind"`
}

// FindP2shAddressInfo find bridge p2sh address info
func FindP2shAddressInfo(dbname, p2shAddress string) (string, error) {
        client := params.GetServerDbClient()
        if client == nil {
                return "", errors.New("mongodb client is nil")
        }
        tablename := tbP2shAddresses
        database := client.Database(dbname)
        c := database.Collection(tablename)
	var result p2shAddressConfig
        err := c.FindOne(clientCtx, bson.M{"p2shaddress": p2shAddress}).Decode(&result)
        if err != nil {
		log.Debug("FindP2shAddressInfo", "p2shAddress", p2shAddress, "result", result, "err", err)
                return "", err
        }
        log.Info("FindP2shAddressInfo", "p2shAddress", p2shAddress, "result", result, "err", err)
	return result.Id, nil
}

