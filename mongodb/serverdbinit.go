// Package mongodb is a wrapper of mongo-go-driver that
// defines the collections and CRUD apis on them.
package mongodb

import (
	"context"
	"sync"
	"time"

	"github.com/anyswap/CrossChain-Bridge/cmd/utils"
	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/weijun-sh/gethscan/params"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	clientCtx = context.Background()

	appIdentifier string
	databaseName  string

	// MgoWaitGroup wait all mongodb related task done
	MgoWaitGroup = new(sync.WaitGroup)
)

//// HasClient has client
//func HasClient() bool {
//	return client != nil
//}

// MongoServerInit int mongodb server session
func MongoServerInit(appName string, hosts []string, dbName, user, pass string) (*mongo.Client) {
	appIdentifier = appName
	databaseName = dbName

	clientOpts := &options.ClientOptions{
		AppName: &appName,
		Hosts:   hosts,
		Auth: &options.Credential{
			AuthSource: dbName,
			Username:   user,
			Password:   pass,
		},
	}

	client, err := connect(clientOpts)
	if err != nil {
		log.Fatal("[mongodb] connect database failed", "hosts", hosts, "dbName", dbName, "appName", appName, "err", err)
	}

	log.Info("[mongodb] connect database success", "hosts", hosts, "dbName", dbName, "appName", appName)

	utils.TopWaitGroup.Add(1)
	go utils.WaitAndCleanup(doCleanup)
	return client
}

func doCleanup() {
	defer utils.TopWaitGroup.Done()
	MgoWaitGroup.Wait()

	client := params.GetServerDbClient()
	err := client.Disconnect(clientCtx)
	if err != nil {
		log.Error("[mongodb] close connection failed", "err", err)
	} else {
		log.Info("[mongodb] close connection success")
	}
}

func connect(opts *options.ClientOptions) (client *mongo.Client, err error) {
	ctx, cancel := context.WithTimeout(clientCtx, 10*time.Second)
	defer cancel()

	client, err = mongo.Connect(ctx, opts)
	if err != nil {
		return client, err
	}

	err = client.Ping(clientCtx, nil)
	if err != nil {
		return client, err
	}

	//initCollections()
	return client, nil
}

