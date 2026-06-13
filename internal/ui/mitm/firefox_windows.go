//go:build windows

package mitm

import "golang.org/x/sys/windows/registry"

const firefoxPolicyKey = `SOFTWARE\Policies\Mozilla\Firefox\Certificates`
const firefoxPolicyValue = "ImportEnterpriseRoots"

func FirefoxEnterpriseRootsEnabled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, firefoxPolicyKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close() //nolint:errcheck
	v, _, err := k.GetIntegerValue(firefoxPolicyValue)
	return err == nil && v == 1
}
