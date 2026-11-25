package istio

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"helm.sh/helm/v3/pkg/chart/loader"
	"istio.io/istio/operator/pkg/component"
	"istio.io/istio/operator/pkg/helm"
	"istio.io/istio/operator/pkg/render"
	"istio.io/istio/operator/pkg/values"
	"pluma.io/pluma-operator/internal/pkg/schema"
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

func stageLegacyGatewayPackage(installPath string) (_ string, err error) {
	source, err := filepath.Abs(installPath)
	if err != nil {
		return "", err
	}

	paths := []string{"profiles"}
	fixedSchemas := make(map[string][]byte)
	for _, c := range component.AllComponents {
		path := filepath.Join("charts", c.HelmSubdir)
		paths = append(paths, path)
		if !c.IsGateway() {
			continue
		}
		cp, loadErr := loader.LoadDir(filepath.Join(source, path))
		if loadErr != nil {
			// The original render error is preserved if no legacy schema matches.
			continue
		}
		fixed, err := schema.FixLegacyGatewaySchema(cp)
		if err != nil {
			return "", err
		}
		if fixed {
			fixedSchemas[path] = cp.Schema
		}
	}
	if len(fixedSchemas) == 0 {
		return "", nil
	}

	staged, err := os.MkdirTemp("", "pluma-install-package-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(staged)
		}
	}()
	// The SDK opens each component chart by its root path. Copy the gateway
	// charts that need changes and reuse the other roots and profiles.
	for _, path := range paths {
		src, dst := filepath.Join(source, path), filepath.Join(staged, path)
		if data, fixed := fixedSchemas[path]; fixed {
			if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
				return "", err
			}
			schemaPath := filepath.Join(dst, "values.schema.json")
			// The copied file can be a symlink. Replace the link itself.
			if err := os.Remove(schemaPath); err != nil {
				return "", err
			}
			if err := os.WriteFile(schemaPath, data, 0o600); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.Symlink(src, dst); err != nil {
			return "", err
		}
	}
	return staged, nil
}

// The Istio SDK removed this component, but older install packages can still provide its chart.
func migrateIstiodRemote(file string, flags []string, components []render.ComponentMigration) ([]render.ComponentMigration, error) {
	merged, err := render.MergeInputs([]string{file}, flags, nil)
	if err != nil {
		return nil, err
	}
	remote := component.Component{
		SpecName:             istiodRemoteSpecName,
		HelmSubdir:           "istiod-remote",
		ToHelmValuesTreeRoot: "global",
		ReleaseName:          "istiod-remote",
	}
	specs, err := remote.Get(merged)
	if err != nil || len(specs) == 0 {
		return components, err
	}
	if overrides, ok := merged.GetPathMap("spec.unvalidatedValues"); ok {
		merged.MergeFrom(values.MakeMap(overrides, "spec", "values"))
	}
	spec := specs[0]
	if spec.Hub != "" {
		if err := merged.SetSpecPaths("values.global.hub=" + spec.Hub); err != nil {
			return nil, err
		}
	}
	if spec.Tag != nil {
		if err := merged.SetSpecPaths(fmt.Sprintf("values.global.tag=%v", spec.Tag)); err != nil {
			return nil, err
		}
	}
	if _, _, err := helm.Render("istio", spec.Namespace, remote.HelmSubdir, merged, nil); err != nil {
		return nil, fmt.Errorf("render %s: %w", remote.ReleaseName, err)
	}
	migration := render.ComponentMigration{Component: remote, ComponentSpec: spec, Values: merged}
	for i, entry := range components {
		if entry.Component.SpecName == ztunnelSpecName {
			return slices.Insert(components, i, migration), nil
		}
	}
	return append(components, migration), nil
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
