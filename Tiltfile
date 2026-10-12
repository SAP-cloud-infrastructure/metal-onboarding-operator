# Dev environment: metal-onboarding-operator on a local kind cluster.
# Bring-up:  make tilt-up
# Tear-down: make tilt-down

allow_k8s_contexts("kind-metal-onboarding-operator")

# Use the local registry created by hack/kind-with-registry.sh.
reg_port = "5001"
reg_host = "localhost:%s" % reg_port
def image_ref(name):
    return "%s/%s" % (reg_host, name)

# --- CRDs -----------------------------------------------------------------------
# DHCPLease CRD is owned by metaldhcp; we carry a copy for local dev.
k8s_yaml("config/crd/external/dhcp.metal.ironcore.dev_dhcpleases.yaml")

k8s_resource(
    new_name = "crds",
    objects = [
        "dhcpleases.dhcp.metal.ironcore.dev:customresourcedefinition",
    ],
)

# --- metal-onboarding-operator --------------------------------------------------
arch = str(local("go env GOARCH", quiet=True)).strip()
docker_build(
    image_ref("metal-onboarding-operator"),
    ".",
    build_args = {"TARGETARCH": arch, "TARGETOS": "linux"},
)

k8s_yaml(helm(
    "./charts/metal-onboarding-operator",
    values = ["./dev/values.yaml"],
))
k8s_resource("metal-onboarding-operator", resource_deps = ["crds"])

