// prohod-e2e: headless dev tool — connect without UI, print status, wait for signal. Used for crash/guard tests.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"prohod/internal/core"
	"prohod/internal/guard"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == guard.Flag {
		guard.Run(os.Args[2:])
		return
	}
	ctl, err := core.New()
	if err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	ctl.StartGuard()
	if err := ctl.Connect(); err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	time.Sleep(4 * time.Second)
	fmt.Printf("status: %+v\n", ctl.Status())
	fmt.Println("READY")
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctl.Shutdown()
}
