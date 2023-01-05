package worker

import (
	"fmt"
	"time"

	"github.com/weijun-sh/gethscan/mongodb"
	"github.com/weijun-sh/gethscan/params"
)

const interval = 10 * time.Millisecond

// StartSwapWork start mongo query job
func StartSwapWork() {
	fmt.Printf("query worker start\n")

	initServerDbClient()
	return
}

func initServerDbClient() {
	dbConfig := params.GetServerDbsConfig()
	client := mongodb.MongoServerInit(
		"servermongodb",
		dbConfig.DBURLs,
		dbConfig.DBName,
		dbConfig.UserName,
		dbConfig.Password,
	)
	params.SetServerDbClient(client)
}

