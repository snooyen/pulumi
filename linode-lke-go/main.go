package main

import (
	"encoding/base64"
	"fmt"

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

		// Deploy kube-prometheus Monitoring Stack
		_, err = kustomize.NewDirectory(ctx, "kube-prometheus",
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

		// Deploy Loki & Promtail
		lokiChart := "loki-stack"
		lokiChartRepo := "https://grafana.github.io/helm-charts"
		lokiNamespace := "logging"
		_, err = helm.NewRelease(ctx, lokiChart,
			&helm.ReleaseArgs{
				Chart: pulumi.String(lokiChart),
				RepositoryOpts: helm.RepositoryOptsArgs{
					Repo: pulumi.String(lokiChartRepo),
				},
				Name:            pulumi.String(lokiChart),
				Namespace:       pulumi.String(lokiNamespace),
				CreateNamespace: pulumi.Bool(true),
				Values:          pulumi.Map{},
			},
			pulumi.ProviderMap(map[string]pulumi.ProviderResource{
				"kubernetes": k8s,
			}),
		)
		if err != nil {
			return err
		}

		// Deploy ExternalDNS
		linodeDNSToken := conf.RequireSecret("linodeDNSToken") // Linode API Token for DNS Managment
		externalDNSChart := "external-dns"
		externalDNSChartRepo := "https://kubernetes-sigs.github.io/external-dns"
		externalDNSChartVersion := "1.3.2"
		externalDNSNamespace := "kube-system"
		_, err = helm.NewRelease(ctx, externalDNSChart,
			&helm.ReleaseArgs{
				Chart: pulumi.String(externalDNSChart),
				RepositoryOpts: helm.RepositoryOptsArgs{
					Repo: pulumi.String(externalDNSChartRepo),
				},
				Name:            pulumi.String(externalDNSChart),
				Namespace:       pulumi.String(externalDNSNamespace),
				CreateNamespace: pulumi.Bool(true),
				Version:         pulumi.String(externalDNSChartVersion),
				Values: pulumi.Map{
					"provider": pulumi.String("linode"),
					"env": pulumi.Array{
						pulumi.Map{
							"name":  pulumi.String("LINODE_TOKEN"),
							"value": linodeDNSToken,
						},
					},
				},
			},
			pulumi.ProviderMap(map[string]pulumi.ProviderResource{
				"kubernetes": k8s,
			}),
		)
		if err != nil {
			return err
		}

		// Deploy Traefik Ingress
		traefikChart := "traefik"
		traefikChartRepo := "https://helm.traefik.io/traefik"
		traefikChartVersion := "10.3.6"
		traefikNamespace := "traefik-v2"
		traefikWebSecureMainDomain := "seannguyen.dev"
		traefikWebSecureWildCardSAN := "*.seannguyen.dev"
		traefikLinodeACMECertResolver := struct {
			Name                 string
			Email                string
			Storage              string
			CAServer             string
			DNSChallengeProvider string
		}{
			Name:                 "linodeACME",
			Email:                "nguyensean95@gmail.com",
			Storage:              "/data/acme.json",
			CAServer:             "https://acme-v02.api.letsencrypt.org/directory",
			DNSChallengeProvider: "linode",
		}
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
				Values: pulumi.Map{
					// The "volume-permissions" init container is required if you run into permission issues.
					// Related issue: https://github.com/traefik/traefik/issues/6972
					"deployment": pulumi.Map{
						"initContainers": pulumi.Array{
							pulumi.Map{
								"name":  pulumi.String("volume-permissions"),
								"image": pulumi.String("busybox:1.31.1"),
								"command": pulumi.StringArray{
									pulumi.String("sh"),
									pulumi.String("-c"),
									pulumi.String("chmod -Rv 600 /data/*"),
								},
								"volumeMounts": pulumi.Array{
									pulumi.Map{
										"name":      pulumi.String("data"),
										"mountPath": pulumi.String("/data"),
									},
								},
							},
						},
					},
					"additionalArguments": pulumi.StringArray{
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s=true", traefikLinodeACMECertResolver.Name)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.email=%s", traefikLinodeACMECertResolver.Name, traefikLinodeACMECertResolver.Email)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.storage=%s", traefikLinodeACMECertResolver.Name, traefikLinodeACMECertResolver.Storage)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.caserver=%s", traefikLinodeACMECertResolver.Name, traefikLinodeACMECertResolver.CAServer)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.tlschallenge=false", traefikLinodeACMECertResolver.Name)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.httpchallenge=false", traefikLinodeACMECertResolver.Name)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.dnschallenge=true", traefikLinodeACMECertResolver.Name)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.dnschallenge.provider=%s", traefikLinodeACMECertResolver.Name, traefikLinodeACMECertResolver.DNSChallengeProvider)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.dnschallenge.delaybeforecheck=5", traefikLinodeACMECertResolver.Name)),
						pulumi.String(fmt.Sprintf("--certificatesresolvers.%s.acme.dnschallenge.resolvers=1.1.1.1:53,8.8.8.8:53", traefikLinodeACMECertResolver.Name)),
					},
					"persistence": pulumi.Map{
						"enabled": pulumi.Bool(true),
					},
					"ports": pulumi.Map{
						"websecure": pulumi.Map{
							"tls": pulumi.Map{
								"enabled":      pulumi.Bool(true),
								"certResolver": pulumi.String(traefikLinodeACMECertResolver.Name),
								"domains": pulumi.MapArray{
									pulumi.Map{
										"main": pulumi.String(traefikWebSecureMainDomain),
										"sans": pulumi.StringArray{
											pulumi.String(traefikWebSecureWildCardSAN),
										},
									},
								},
							},
						},
					},
					"providers": pulumi.Map{
						"kubernetesIngress": pulumi.Map{
							"publishedService": pulumi.Map{
								"enabled": pulumi.Bool(true),
							},
						},
					},
					"ingressClass": pulumi.Map{
						"enabled":            pulumi.Bool(true),
						"isDefaultClass":     pulumi.Bool(true),
						"fallbackApiVersion": pulumi.String("v1"),
					},
					"logs": pulumi.Map{
						"general": pulumi.Map{
							"level": pulumi.String("ERROR"),
						},
						"access": pulumi.Map{
							"enabled": pulumi.Bool(true),
						},
					},
					"env": pulumi.Array{
						pulumi.Map{
							"name":  pulumi.String("LINODE_TOKEN"),
							"value": linodeDNSToken,
						},
					},
				},
			},
			pulumi.ProviderMap(map[string]pulumi.ProviderResource{
				"kubernetes": k8s,
			}),
		)
		if err != nil {
			return err
		}

		// Grafana Ingress
		_, err = kustomize.NewDirectory(ctx, "grafana",
			kustomize.DirectoryArgs{
				Directory: pulumi.String("./manifests/base"),
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
