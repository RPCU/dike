{
  pkgs,
  lib,
  ...
}:
{
  packages = with pkgs; [
    go
    golangci-lint
    kustomize
    kubernetes-helm
    kubectl
    kind
  ];

  enterShell = ''
    echo ""
    echo "Dike development environment loaded"
    echo ""
  '';
}
