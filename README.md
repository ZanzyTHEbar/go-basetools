# Go Base Tools

This repository contains a collection of base tools for Go, including reactive entities, data structures, and utilities. The tools are designed to be lightweight, efficient, and easy to use, providing essential functionality for various applications.

This repository is a work-in-progress, and new tools will be added over time. Feel free to contribute, suggest improvements, or request specific tools that you'd like to see added.

## Tools

- [Reactive Library](#reactive-library)

### Reactive Library

This lightweight reactive library provides reactive entities for Go, allowing you to manage subscribers and publish updates in a way that adheres to Go's philosophy of simplicity, channels, and concurrency. Subscribers receive updates through channels, and you can target specific subscribers using the `Notify` method.

#### Methods

##### `Subscribe(id string) (<-chan T, error)`

Creates a new subscriber with a unique `id`. Returns a channel that receives messages. If a subscriber with the given ID already exists, returns an error.

##### `Unsubscribe(id string) error`

Unsubscribes the subscriber with the given `id`. After this, the subscriber will no longer receive messages.

##### `Broadcast(value T)`

Sends a message to all subscribers.

##### `Notify(id string, value T) error`

Sends a message to a specific subscriber, identified by `id`. If no such subscriber exists, returns an error.

##### `Close()`

Closes all subscriber channels and clears the map of subscribers, cleaning up resources.
