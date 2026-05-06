//go:build ignore

package main

import (
	"fmt"
	
	"github.com/clerk/clerk-sdk-go/v2"
)

func main() {
	// Test Clerk SDK initialization
	fmt.Println("Testing Clerk SDK initialization...")
	
	// Try to set the key
	clerk.SetKey("sk_test_lwJjXRW6U6BEe76zqtseyBfC73YgSxYRt3GH5hYrC0")
	
	// Try to get the key
	key := clerk.GetBackend()
	if key == nil {
		fmt.Println("ERROR: Clerk backend is nil")
	} else {
		fmt.Println("SUCCESS: Clerk backend is initialized")
	}
}
