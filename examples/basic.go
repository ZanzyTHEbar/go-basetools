package main

import (
    "context"
    "fmt"
    "time"
    "github.com/ZanzyTHEbar/go-basetools"
)

func main() {
    obs := gobasetools.NewObservable[string]()
    ctx := context.Background()

    // Subscribe with unique IDs using context
    subA, _ := obs.Subscribe(ctx, "A")
    subB, _ := obs.Subscribe(ctx, "B")

    // Subscriber A goroutine
    go func() {
        for msg := range subA {
            fmt.Println("Subscriber A received:", msg)
        }
        fmt.Println("Subscriber A channel closed")
    }()

    // Subscriber B goroutine
    go func() {
        for msg := range subB {
            fmt.Println("Subscriber B received:", msg)
        }
        fmt.Println("Subscriber B channel closed")
    }()

    // Give goroutines time to start
    time.Sleep(10 * time.Millisecond)

    // Broadcast to all subscribers
    obs.Broadcast("Hello, all!")

    // Notify specific subscribers
    obs.Notify("A", "Hello, A only!")
    obs.Notify("B", "Hello, B only!")

    // Give time for messages to be processed
    time.Sleep(50 * time.Millisecond)

    // Unsubscribe and close
    obs.Unsubscribe("A")
    obs.Unsubscribe("B")
    
    // Give time for unsubscribe to complete
    time.Sleep(50 * time.Millisecond)
    
    obs.Close()
    
    // Give time for final cleanup
    time.Sleep(50 * time.Millisecond)
    fmt.Println("Example completed")
}
