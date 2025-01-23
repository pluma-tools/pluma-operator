package istio

import (
	"istio.io/istio/operator/pkg/component"
	"istio.io/istio/operator/pkg/render"
)

type IOPComponent struct {
	component.Component

	HelmChartName string
	// Compatible with istio legacy, the root node of component values
	HelmBaseRootKey string
}

const (
	baseSpecName         = "base"
	pilotSpecName        = "pilot"
	ingressSpecName      = "ingressGateways"
	egressSpecName       = "egressGateways"
	cniSpecName          = "cni"
	istiodRemoteSpecName = "istiodRemote"
	ztunnelSpecName      = "ztunnel"
)

func isIngressGateway(c render.ComponentMigration) bool {
	return c.Component.SpecName == ingressSpecName
}

func isEgressGateway(c render.ComponentMigration) bool {
	return c.Component.SpecName == egressSpecName
}

func isGateway(c render.ComponentMigration) bool {
	return isIngressGateway(c) || isEgressGateway(c)
}

// getComponent returns IOPComponent for the given specName
func getComponent(specName string) IOPComponent {
	for _, c := range component.AllComponents {
		if c.SpecName == specName {
			comp := IOPComponent{
				Component:     c,
				HelmChartName: c.ReleaseName,
			}

			switch specName {
			case ingressSpecName, egressSpecName:
				comp.HelmChartName = "gateway"
			case pilotSpecName, cniSpecName:
				comp.HelmBaseRootKey = c.ToHelmValuesTreeRoot
			}

			return comp
		}
	}

	return IOPComponent{}
}
