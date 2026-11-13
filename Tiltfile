# Dev environment: metaldhcp + metal-onboarding-operator on a local kind cluster.
# Bring-up:  make tilt-up
# Tear-down: make tilt-down

allow_k8s_contexts("kind-metal-onboarding-operator")

# Use the local registry created by hack/kind-with-registry.sh.
reg_port = "5001"
reg_host = "localhost:%s" % reg_port
def image_ref(name):
    return "%s/%s" % (reg_host, name)

# --- CRDs -----------------------------------------------------------------------
# Both CRDs are owned by metaldhcp; we carry copies for local dev.
k8s_yaml([
    "config/crd/external/dhcp.metal.ironcore.dev_dhcpleases.yaml",
    "config/crd/external/dhcp.metal.ironcore.dev_oobsubnets.yaml",
])

k8s_resource(
    new_name = "crds",
    objects = [
        "dhcpleases.dhcp.metal.ironcore.dev:customresourcedefinition",
        "oobsubnets.dhcp.metal.ironcore.dev:customresourcedefinition",
    ],
)

# --- metaldhcp ------------------------------------------------------------------
metaldhcp_src = "/Users/d058695/go/pkg/mod/github.com/!s!a!p-cloud-infrastructure/metaldhcp@v0.0.0-20261013111444-a66535c0428a"

arch = str(local("go env GOARCH", quiet=True)).strip()
docker_build(
    image_ref("metaldhcp"),
    metaldhcp_src,
    dockerfile = metaldhcp_src + "/Dockerfile.dev",
    build_args = {"TARGETARCH": arch, "TARGETOS": "linux"},
)

k8s_yaml(helm(
    metaldhcp_src + "/chart/metaldhcp",
    values = ["./dev/values-metaldhcp.yaml"],
))
k8s_resource("metaldhcp", resource_deps = ["crds"])

# --- fake-redfish BMC server ----------------------------------------------------
# Serves GET /redfish/v1 over HTTPS so moo can exercise the serial-first lookup
# path locally. ServiceTag/ManagerType are set via env vars in dev/fake-redfish.yaml.
docker_build(
    image_ref("fake-redfish"),
    ".",
    dockerfile = "dev/fake-redfish/Dockerfile",
    build_args = {"TARGETARCH": arch, "TARGETOS": "linux"},
    only = ["dev/fake-redfish/main.go"],
)
k8s_yaml("dev/fake-redfish.yaml")
k8s_resource("fake-redfish", resource_deps = ["crds"])

# --- metal-onboarding-operator --------------------------------------------------
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
