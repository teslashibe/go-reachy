#!/bin/bash
# Voice Pipeline Integration Test Runner
# 
# Quick way to test and compare latency across all voice pipelines.
#
# Usage:
#   ./scripts/test-voice.sh openai      # Test OpenAI Realtime
#   ./scripts/test-voice.sh gemini      # Test Gemini Live  
#   ./scripts/test-voice.sh elevenlabs  # Test ElevenLabs
#   ./scripts/test-voice.sh all         # Test all providers
#   ./scripts/test-voice.sh compare     # Run comparison test

set -e

cd "$(dirname "$0")/.."

PROVIDER="${1:-openai}"
LOOPS="${2:-3}"
DURATION="${3:-2s}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════${NC}"
echo -e "${BLUE}    Voice Pipeline Integration Tester${NC}"
echo -e "${BLUE}═══════════════════════════════════════════${NC}"
echo

# Check for required environment variables
check_env() {
    local provider=$1
    case $provider in
        openai)
            if [ -z "$OPENAI_API_KEY" ]; then
                echo -e "${RED}Error: OPENAI_API_KEY not set${NC}"
                return 1
            fi
            ;;
        gemini)
            if [ -z "$GOOGLE_API_KEY" ]; then
                echo -e "${RED}Error: GOOGLE_API_KEY not set${NC}"
                return 1
            fi
            ;;
        elevenlabs)
            if [ -z "$ELEVENLABS_API_KEY" ]; then
                echo -e "${RED}Error: ELEVENLABS_API_KEY not set${NC}"
                return 1
            fi
            if [ -z "$ELEVENLABS_VOICE_ID" ]; then
                echo -e "${RED}Error: ELEVENLABS_VOICE_ID not set${NC}"
                return 1
            fi
            ;;
    esac
    return 0
}

run_test() {
    local provider=$1
    echo -e "${YELLOW}Testing: ${provider}${NC}"
    echo "----------------------------------------"
    
    if ! check_env "$provider"; then
        echo -e "${RED}Skipping ${provider} (missing credentials)${NC}"
        echo
        return
    fi
    
    go run ./cmd/test-voice \
        --provider "$provider" \
        --loops "$LOOPS" \
        --duration "$DURATION"
    
    echo
}

case $PROVIDER in
    openai|gemini|elevenlabs)
        run_test "$PROVIDER"
        ;;
    all)
        echo -e "${GREEN}Running all provider tests...${NC}"
        echo
        run_test "openai"
        run_test "gemini"
        run_test "elevenlabs"
        ;;
    compare)
        echo -e "${GREEN}Running comparison test (1 loop each)...${NC}"
        echo
        LOOPS=1
        
        echo "═══════════════════════════════════════════"
        echo "PROVIDER COMPARISON"
        echo "═══════════════════════════════════════════"
        
        for p in openai gemini elevenlabs; do
            if check_env "$p" 2>/dev/null; then
                echo -e "\n${BLUE}=== $p ===${NC}"
                go run ./cmd/test-voice --provider "$p" --loops 1 --duration 2s 2>&1 | grep -E "Pipeline|Total|Error"
            fi
        done
        
        echo
        echo "═══════════════════════════════════════════"
        ;;
    *)
        echo "Usage: $0 <provider> [loops] [duration]"
        echo
        echo "Providers:"
        echo "  openai      - Test OpenAI Realtime API"
        echo "  gemini      - Test Gemini Live API"
        echo "  elevenlabs  - Test ElevenLabs Conversational AI"
        echo "  all         - Test all providers"
        echo "  compare     - Quick comparison (1 loop each)"
        echo
        echo "Examples:"
        echo "  $0 openai 5 3s    # 5 loops, 3 second audio"
        echo "  $0 gemini         # Default: 3 loops, 2 second audio"
        echo "  $0 compare        # Quick comparison of all"
        exit 1
        ;;
esac

echo -e "${GREEN}Done!${NC}"

