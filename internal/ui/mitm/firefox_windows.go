//go:build windows

package mitm

import "golang.org/x/sys/windows/registry"

const firefoxPolicyKey = `SOFTWARE\Policies\Mozilla\Firefox\Certificates`
const firefoxPolicyValue = "ImportEnterpriseRoots"

// FirefoxEnterpriseRootsEnabled reports whether the Firefox enterprise-
// roots policy is currently set so Firefox will import roots from the
// Windows trust store. Read-only — we never write to Firefox's policy
// registry, that's the user's call. We only check the state so the UI
// can show whether a manual import into Firefox is still required.
func FirefoxEnterpriseRootsEnabled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, firefoxPolicyKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(firefoxPolicyValue)
	return err == nil && v == 1
}
