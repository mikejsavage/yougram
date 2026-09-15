package main

// this is a simple background task runner, with a fast task queue and a slow
// task queue. the fast queue is prioritised over the slow queue, but fast
// tasks do not preempt slow tasks

import (
	"sync"
)

var fast_tasks []func()
var slow_tasks []func()

var wake_channel chan int
var shutdown bool
var shutdown_waiter sync.WaitGroup

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
}

func shutdownBackgroundTaskRunner() {
	shutdown = true
	wakeTheTaskRunner()
	shutdown_waiter.Wait()
}

func wakeTheTaskRunner() {
	select {
    case wake_channel <- 1:
    default:
    }
}

func addFastBackgroundTask( task func() ) {
	fast_tasks = append( fast_tasks, task )
	wakeTheTaskRunner()
}

func addSlowBackgroundTask( task func() ) {
	slow_tasks = append( slow_tasks, task )
	wakeTheTaskRunner()
}
