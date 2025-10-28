package main

import (
	"context"
	"fmt"
	"sync"

	"mikegram/moondream"
	"mikegram/sqlc"
)

var fast_tasks []func()
var slow_tasks []func()

var wake_channel chan int
var shutdown bool
var shutdown_waiter sync.WaitGroup

const (
	TagGenerator_0 = iota // lobotomised Moondream 2b from moondream-zig
)

func tagAPhoto() {
	if !moondream.Ok() {
		return
	}

	untagged := queryOptional( queries.GetAnAssetThatNeedsANewAIDescription( context.Background(), 0 ) )
	if !untagged.Valid {
		return
	}

	description := moondream.DescribePhoto( untagged.V.Thumbnail )
	must( queries.SetAssetAIDescription( context.Background(), sqlc.SetAssetAIDescriptionParams {
		AssetID: untagged.V.Sha256,
		Generator: TagGenerator_0,
		Description: description,
	} ) )

	fmt.Printf( "%v -> %s\n", untagged.V.Sha256, description )

	addSlowBackgroundTask( tagAPhoto )
}

// TODO: maybe try to thread a context through this
func initBackgroundTaskRunner() {
	shutdown = false
	wake_channel = make( chan int )

	shutdown_waiter.Add( 1 )
	go func() {
		for {
			<- wake_channel
			for {
				if shutdown {
					shutdown_waiter.Done()
					return
				}
				if len( fast_tasks ) == 0 && len( slow_tasks ) == 0 {
					break
				}

				var task func()
				if len( fast_tasks ) > 0 {
					task = fast_tasks[ 0 ]
					fast_tasks = fast_tasks[ 1: ]
				} else {
					task = slow_tasks[ 0 ]
					slow_tasks = slow_tasks[ 1: ]
				}

				task()
			}
		}
	}()

	// wait until the task runner is ready
	wake_channel <- 1

	addSlowBackgroundTask( tagAPhoto )
}

func wakeTheTaskRunner() {
	select {
    case wake_channel <- 1:
    default:
    }
}

func shutdownBackgroundTaskRunner() {
	shutdown = true
	wakeTheTaskRunner()
	shutdown_waiter.Wait()
}

func addFastBackgroundTask( task func() ) {
	fast_tasks = append( fast_tasks, task )
	wakeTheTaskRunner()
}

func addSlowBackgroundTask( task func() ) {
	slow_tasks = append( slow_tasks, task )
	wakeTheTaskRunner()
}
