package mongodb

import (
//"gopkg.in/mgo.v2/bson"
)

const (
	tbSwap        string = "scanSwap"
	tbSwapPending string = "ScanPending"
	tbSwapPendingAfterPending string = "scanPendingAfterPeriod"
	tbSwapDeleted string = "ScanDeleted"
	tbSyncedBlock string = "syncedBlock"
)

type MgoSwap struct {
	Id         string `bson:"_id"` //chainid:txid:tochainid
	Txid       string `bson:"txid"` //txid
	PairID     string `bson:"pairID"`    //"FXSv4"
	RpcMethod  string `bson:"rpcMethod"` //"swap.Swapin"
	SwapServer string `bson:"swapServer"`
        ChainID    string `bson:"chainid"`
        ToChainID  string `bson:"tochainid"`
        LogIndex   string `bson:"logIndex"`
	Chain      string `bson:"chain"`
	Timestamp  uint64 `bson:"timestamp"`
}

type SyncedBlock struct {
	Id          string `bson: "_id"` //"chain"
	Chain       string `bson: "chain"`
	BlockNumber uint64 `bson: "blocknumber"`
}

