package main

import (
	"encoding/base64"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/kustomize"
	"github.com/pulumi/pulumi-linode/sdk/v3/go/linode"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	monitoringManifests = `https://github.com/prometheus-operator/kube-prometheus/tree/release-0.9`
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

		// Initialize K8s Provider
		//
		kubeconfig := cluster.Kubeconfig.ApplyT(func(b64EncKubeconfig string) string {
			data, _ := base64.StdEncoding.DecodeString(b64EncKubeconfig)
			return string(data)
		}).(pulumi.StringOutput)

		k8s, err := kubernetes.NewProvider(ctx, clusterArgs.Name, &kubernetes.ProviderArgs{
			Kubeconfig: kubeconfig,
		})
		if err != nil {
			return err
		}

		// Deploy Monitoring Stacks
		_, err = kustomize.NewDirectory(
			ctx,
			"kube-prometheus",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(monitoringManifests),
			},
			pulumi.ProviderMap(map[string]pulumi.ProviderResource{
    			"kubernetes": k8s,
			}),
		)
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
