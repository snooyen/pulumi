package main

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
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
		k8s, err := kubernetes.NewProvider(ctx, clusterArgs.Name, &kubernetes.ProviderArgs{
			Kubeconfig: cluster.Kubeconfig,
		})
		if err != nil {
			return err
		}

		// Deploy kube-prometheus Monitoring Stack
		_, err = kustomize.NewDirectory(ctx, "kube-prometheus",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(monitoringManifests),
			},
			pulumi.Provider(k8s),
		)
		if err != nil {
			return err
		}

		// Deploy Traefik Ingress
		traefikChart := "traefik"
		traefikChartRepo := "https://helm.traefik.io/traefik"
		traefikChartVersion := "10.3.6"
		traefikNamespace := "traefik-v2"
		_, err = helm.NewRelease(ctx, traefikChart,
			&helm.ReleaseArgs{
				Chart: pulumi.String(traefikChart),
				RepositoryOpts: helm.RepositoryOptsArgs{
					Repo: pulumi.String(traefikChartRepo),
				},
				Name:            pulumi.String(traefikChart),
				Namespace:       pulumi.String(traefikNamespace),
				CreateNamespace: pulumi.Bool(true),
				Version:         pulumi.String(traefikChartVersion),
				Values:          pulumi.Map{},
			},
			pulumi.Provider(k8s),
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
