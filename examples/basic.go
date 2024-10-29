package main

import (
    "fmt"
    "github.com/ZanzyTHEbar/go-basetools"
)

func main() {
    obs := gobasetools.NewObservable[string]()

    // Subscribe with unique IDs
    subA, _ := obs.Subscribe("A")
    subB, _ := obs.Subscribe("B")

    // Subscriber A goroutine
    go func() {
        for msg := range subA {
            fmt.Println("Subscriber A received:", msg)
        }
    }()

    // Subscriber B goroutine
    go func() {
        for msg := range subB {
            fmt.Println("Subscriber B received:", msg)
        }
    }()

    // Broadcast to all subscribers
    obs.Broadcast("Hello, all!")

    // Notify specific subscribers
    obs.Notify("A", "Hello, A only!")
    obs.Notify("B", "Hello, B only!")

    // Unsubscribe and close
    obs.Unsubscribe("A")
    obs.Unsubscribe("B")
    obs.Close()
}
