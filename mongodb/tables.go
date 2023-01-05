package mongodb

import (
//"gopkg.in/mgo.v2/bson"
)

const (
	tbSwap        string = "swap"
	tbSwapPending string = "pending"
	tbSwapDeleted string = "deleted"
	tbSyncedBlock string = "syncedBlock"

	tbP2shAddresses string = "P2shAddresses"
)

type MgoSwap struct {
	Id         string `bson:"_id"` //txid
	PairID     string `bson:"pairID"`    //"FXSv4"
	Bind       string `bson:"bind"`      //"bind"
	RpcMethod  string `bson:"rpcMethod"` //"swap.Swapin"
	SwapServer string `bson:"swapServer"`
        ChainID    string `bson:"chainid"`
        LogIndex   string `bson:"logIndex"`
	Chain      string `bson:"chain"`
	Timestamp  uint64 `bson:"timestamp"`
}

type SyncedBlock struct {
	Id          string `bson: "_id"` //"chain"
	Chain       string `bson: "chain"`
	BlockNumber uint64 `bson: "blocknumber"`
}

