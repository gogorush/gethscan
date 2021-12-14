package mongodb

import (
	"errors"
	"time"

	"github.com/anyswap/ANYToken-distribution/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
func RemoveSwapPending(id string) (err error) {
	err = collectionSwapPending.Remove(bson.M{"_id": id})
	if err == nil {
		log.Info("[mongodb] RemoveSwapPending success", "pending", id)
	} else {
		log.Warn("[mongodb] RemoveSwapPending failed", "pending", id, "err", err)
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
func FindAllSwapPending(chain string) []*MgoSwap {
	iter := findAllSwapPending(chain)
	var tmp MgoSwap
	var result []*MgoSwap
	for iter.Next(&tmp) {
		sp := deepCopy(&tmp)
		result = append(result, sp)
	}
	return result
}

func deepCopy(swap *MgoSwap) *MgoSwap {
	sp := MgoSwap{}
	sp.Id = swap.Id
	sp.PairID = swap.PairID
	sp.RpcMethod = swap.RpcMethod
	sp.SwapServer = swap.SwapServer
	sp.ChainID = swap.ChainID
	sp.Chain = swap.Chain
	sp.LogIndex = swap.LogIndex
	sp.Timestamp = swap.Timestamp
	return &sp
}

func UpdateSwapPending(swap *MgoSwap) {
	RemoveSwapPending(swap.Id)

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
