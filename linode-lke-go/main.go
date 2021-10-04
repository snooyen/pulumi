package main

import (
	"github.com/pulumi/pulumi-linode/sdk/v3/go/linode"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type LkeClusterArgs struct {
	Name       string
	K8sVersion string
	Region     string
	Tags       []string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// Stack Configs
		var clusterArgs LkeClusterArgs
		conf := config.New(ctx, "")
		conf.RequireObject("clusterArgs", &clusterArgs)

		// Create a new Linode LKE Cluster
		cluster, err := linode.NewLkeCluster(ctx, clusterArgs.Name, &linode.LkeClusterArgs{
			K8sVersion: pulumi.String(clusterArgs.K8sVersion),
			Label:      pulumi.String(clusterArgs.Name),
			Pools: linode.LkeClusterPoolArray{
				&linode.LkeClusterPoolArgs{
					Count: pulumi.Int(3),
					Type:  pulumi.String("g6-standard-1"),
				},
			},
			Region: pulumi.String(clusterArgs.Region),
			Tags:   pulumi.ToStringArray(clusterArgs.Tags),
		})
		if err != nil {
			return err
		}

		// Outputs
		ctx.Export("api_endpoints", cluster.ApiEndpoints)
		ctx.Export("kubeconfig", cluster.Kubeconfig)
		ctx.Export("status", cluster.Status)

		return nil
	})
}
