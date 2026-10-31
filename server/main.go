// Command kandev-plugin-brainstorm is the backend half of this plugin. It is
// spawned by kandev as a gRPC subprocess; pluginsdk.Serve owns the transport.
package main

import "github.com/kandev/kandev/pkg/pluginsdk"

func main() {
	pluginsdk.Serve(newPlugin())
}
