#!/bin/bash

# Notification Service Stress Test Suite (Parallel Execution)
# This script runs all stress test scenarios concurrently.

API_KEYS="nh_lovhtryqqv_LOVHTRYQQVQV3SZRH7F36BRYPLPCUDBMZEXC2TQ5X47XY4CXKN5A,nh_zju4zrtqoi_ZJU4ZRTQOIL6N2O7RI64DB7N6FBJWT766RPMQPPF2WZRD4AW2DLA"
SMS_RECIPIENT="+916382531516"
EMAIL_RECIPIENT="karthikeyan@appointy.com"

echo "🚀 Starting Parallel Stress Test Suite..."

# Run all tests in the background
# echo "Starting SMS - High Priority (5 RPS)..."
# go run main.go -api-keys "$API_KEYS" -channel sms -recipient "$SMS_RECIPIENT" -concurrency 5 -rps 5 -duration 30s -priority high &

echo "Starting Email - High Priority (5 RPS)..."
go run main.go -api-keys "$API_KEYS" -channel email -recipient "$EMAIL_RECIPIENT" -concurrency 5 -rps 5 -duration 30s -priority high &

# echo "Starting SMS - Medium Priority (7 RPS)..."
# go run main.go -api-keys "$API_KEYS" -channel sms -recipient "$SMS_RECIPIENT" -concurrency 5 -rps 7 -duration 30s -priority medium &

echo "Starting Email - Medium Priority (7 RPS)..."
go run main.go -api-keys "$API_KEYS" -channel email -recipient "$EMAIL_RECIPIENT" -concurrency 5 -rps 7 -duration 30s -priority medium &

# echo "Starting SMS - Low Priority (10 RPS)..."
# go run main.go -api-keys "$API_KEYS" -channel sms -recipient "$SMS_RECIPIENT" -concurrency 5 -rps 10 -duration 30s -priority low &

echo "Starting Email - Low Priority (10 RPS)..."
go run main.go -api-keys "$API_KEYS" -channel email -recipient "$EMAIL_RECIPIENT" -concurrency 5 -rps 10 -duration 30s -priority low &

echo "⏳ All tests launched. Waiting for completion..."
wait

echo -e "\n✨ All parallel stress tests completed!"
