package icmpping

// Windows has no unprivileged ICMP socket mode; pro-bing needs raw sockets.
func isRoot() bool { return true }
