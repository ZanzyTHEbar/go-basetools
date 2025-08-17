package main

import (
    "context"
    "fmt"
    "time"
    "github.com/ZanzyTHEbar/go-basetools"
)

func main() {
    fmt.Println("=== Go BaseTools Reactive Library - Advanced Example ===")
    
    // Create observable with default settings
    obs := gobasetools.NewObservable[string]()
    defer obs.Close()
    
    ctx := context.Background()
    
    // Example 1: Basic subscription and broadcasting
    fmt.Println("\n1. Basic Subscription and Broadcasting:")
    sub1, _ := obs.Subscribe(ctx, "basic")
    
    go func() {
        for msg := range sub1 {
            fmt.Printf("   Basic subscriber received: %s\n", msg)
        }
    }()
    
    obs.Broadcast("Hello World!")
    time.Sleep(50 * time.Millisecond)
    
    // Example 2: Subscriber with custom buffer size and drop callback
    fmt.Println("\n2. Custom Buffer and Drop Callback:")
    
    dropCount := 0
    sub2, _ := obs.Subscribe(ctx, "custom", 
        gobasetools.WithBuffer[string](2),
        gobasetools.WithDropCallback[string](func(id string, value string) {
            dropCount++
            fmt.Printf("   DROPPED: %s (total drops: %d)\n", value, dropCount)
        }))
    
    // Don't read from this subscriber to demonstrate backpressure
    _ = sub2
    
    // Send multiple messages to trigger drops
    obs.Notify("custom", "Message 1")
    obs.Notify("custom", "Message 2") 
    obs.Notify("custom", "Message 3") // Should be buffered
    obs.Notify("custom", "Message 4") // Should be dropped
    obs.Notify("custom", "Message 5") // Should be dropped
    
    time.Sleep(50 * time.Millisecond)
    
    // Example 3: Replay latest functionality
    fmt.Println("\n3. Replay Latest Functionality:")
    
    obs.Broadcast("Latest broadcast message")
    time.Sleep(10 * time.Millisecond)
    
    sub3, _ := obs.Subscribe(ctx, "replay", gobasetools.WithReplayLatest[string](true))
    
    go func() {
        msg := <-sub3
        fmt.Printf("   Replay subscriber immediately received: %s\n", msg)
    }()
    
    time.Sleep(50 * time.Millisecond)
    
    // Example 4: Context cancellation
    fmt.Println("\n4. Context Cancellation:")
    
    cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()
    
    _, err := obs.Subscribe(cancelCtx, "timeout")
    if err != nil {
        fmt.Printf("   Subscribe failed as expected: %v\n", err)
    }
    
    // Example 5: Drop oldest policy
    fmt.Println("\n5. Drop Oldest Policy:")
    
    sub5, _ := obs.Subscribe(ctx, "drop-oldest",
        gobasetools.WithBuffer[string](1),
        gobasetools.WithPolicy[string](gobasetools.BackpressureDropOldest),
        gobasetools.WithDropCallback[string](func(id string, value string) {
            fmt.Printf("   DROP OLDEST: %s\n", value)
        }))
    
    // Don't read immediately
    _ = sub5
    
    obs.Notify("drop-oldest", "First")
    obs.Notify("drop-oldest", "Second") // Should cause "First" to be dropped
    
    time.Sleep(50 * time.Millisecond)
    
    // Example 6: Demonstrate clean shutdown
    fmt.Println("\n6. Clean Shutdown:")
    
    fmt.Printf("   Active subscribers: %d\n", obs.Len())
    
    obs.Unsubscribe("basic")
    obs.Unsubscribe("custom") 
    obs.Unsubscribe("replay")
    obs.Unsubscribe("drop-oldest")
    
    time.Sleep(50 * time.Millisecond)
    fmt.Printf("   Subscribers after cleanup: %d\n", obs.Len())
    
    fmt.Println("\n=== Example completed successfully! ===")
}