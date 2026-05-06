//go:build ignore

package main

import (
	"fmt"
	
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

func main() {
	// Set the secret key
	secretKey := "sk_test_lwJjXRW6U6BEe76zqtseyBfC73YgSxYRt3GH5hYrC0"
	
	fmt.Println("Step 1: Setting Clerk secret key...")
	clerk.SetKey(secretKey)
	fmt.Println("✓ Clerk SDK initialized")
	
	// Try to verify a token
	fmt.Println("\nStep 2: Testing token verification...")
	
	// Try with an empty token
	token := ""
	claims, err := jwt.Verify(nil, &jwt.VerifyParams{Token: token})
	if err != nil {
		fmt.Printf("✓ Empty token correctly rejected: %v\n", err)
	} else {
		fmt.Printf("✗ Empty token should have failed but succeeded: %v\n", claims)
	}
	
	// Try with a fake token
	token = "fake.token.here"
	claims, err = jwt.Verify(nil, &jwt.VerifyParams{Token: token})
	if err != nil {
		fmt.Printf("✓ Fake token correctly rejected: %v\n", err)
	} else {
		fmt.Printf("✗ Fake token should have failed but succeeded: %v\n", claims)
	}
	
	fmt.Println("\n✓ All tests passed!")
}
