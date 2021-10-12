package main

import (
	"github.com/pulumi/pulumi-linode/sdk/v3/go/linode"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type DomainArgs struct {
	Name string
	Domain string
	Email string
	Tags []string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Master Domain Configs
		var masterDomainArgs DomainArgs
		conf := config.New(ctx, "masterDomain")
		conf.RequireObject("args", &masterDomainArgs)

		_, err := linode.NewDomain(ctx, masterDomainArgs.Name, &linode.DomainArgs{
			Type:     pulumi.String("master"),
			Domain:   pulumi.String(masterDomainArgs.Domain),
			SoaEmail: pulumi.String(masterDomainArgs.Email),
			Tags: pulumi.ToStringArray(masterDomainArgs.Tags),
		})
		if err != nil {
			return err
		}


		return nil
	})
}
