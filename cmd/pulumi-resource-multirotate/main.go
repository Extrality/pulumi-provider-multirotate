package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumiprovider "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	multirotate "github.com/Extrality/pulumi-provider-multirotate"
)

func main() {
	pulumiprovider.Main("pulumi-resource-multirotate", func(_ *pulumiprovider.HostClient) (pulumirpc.ResourceProviderServer, error) {
		return plugin.NewProviderServer(multirotate.NewMultiRotateProvider()), nil
	})
}
