#!/bin/bash
# optimize_system.sh - Set system parameters for high concurrency zRPC

set -e

YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${YELLOW}zRPC High Concurrency System Optimizer${NC}"
echo "This script will configure your system for high concurrency (10,000+ connections)"
echo

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo -e "${YELLOW}Warning:${NC} This script needs root privileges to set system parameters."
  echo "Please run as root or with sudo."
  exit 1
fi

# Define parameters to set
echo -e "${GREEN}Setting TCP parameters...${NC}"

# Increase file descriptor limits
echo "fs.file-max = 2097152" > /etc/sysctl.d/90-zrpc-limits.conf
echo "fs.nr_open = 2097152" >> /etc/sysctl.d/90-zrpc-limits.conf

# TCP settings for high-concurrency
echo "net.ipv4.tcp_max_syn_backlog = 65536" >> /etc/sysctl.d/90-zrpc-limits.conf
echo "net.core.somaxconn = 65536" >> /etc/sysctl.d/90-zrpc-limits.conf
echo "net.ipv4.ip_local_port_range = 1024 65535" >> /etc/sysctl.d/90-zrpc-limits.conf
echo "net.ipv4.tcp_tw_reuse = 1" >> /etc/sysctl.d/90-zrpc-limits.conf
echo "net.ipv4.tcp_fin_timeout = 30" >> /etc/sysctl.d/90-zrpc-limits.conf
echo "net.core.netdev_max_backlog = 65536" >> /etc/sysctl.d/90-zrpc-limits.conf

# Apply changes
sysctl -p /etc/sysctl.d/90-zrpc-limits.conf

# Set ulimit for the shell
echo -e "${GREEN}Setting file descriptor limits...${NC}"

# Add limits to /etc/security/limits.conf
cat >> /etc/security/limits.conf << EOF
# zRPC high concurrency limits
* soft nofile 1048576
* hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
EOF

echo -e "${GREEN}System parameters set!${NC}"
echo
echo -e "${YELLOW}Note:${NC} You may need to log out and back in for ulimit settings to take effect."
echo "To check current limits, run: ulimit -n"
echo
echo -e "${GREEN}Recommended Go environment settings:${NC}"
echo "export GOMAXPROCS=$(nproc)"
echo "export GOGC=200"
echo
echo -e "${GREEN}Done!${NC}"
