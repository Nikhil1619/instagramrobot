#!/bin/sh

# Start the web service in the background
./web &

# Start the bot service in the foreground
./bot
