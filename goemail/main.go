package goemail

import (
	"fmt"
	"flag"
	"strings"
)

func init() {
	fmt.Printf("goemail init\n")
	serverHost := "smtp.163.com"
	serverPort := 465
	fromEmail := "anyswapinfo@163.com"
	fromPasswd := "ASQMTJHSIOKCECHB"

	myToers := fmt.Sprintf("%v", "anyswapinfo@163.com,zhengxin.gao@anyswap.exchange,xiaozongyuan@163.com") // 逗号隔开
	fmt.Printf("sendto: %v\n", myToers)
	myCCers := ""

	// 结构体赋值
	myEmail := &EmailParam{
		ServerHost: serverHost,
		ServerPort: serverPort,
		FromEmail:  fromEmail,
		FromPasswd: fromPasswd,
		Toers:      myToers,
		CCers:      myCCers,
	}
	InitEmail(myEmail)
}

