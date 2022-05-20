# Theia

Theiactl is the command-line tool for Theia. At the moment, theiactl supports running out-of-cluster only.

## Table of Contents

<!-- toc -->
- [Compile](#compile)
- [Usage](#usage)
<!-- /toc -->
## Compile

On Linux:

```bash
make theiactl
chmod +x ./bin/theiactl
mv ./bin/theiactl /some-dir-in-your-PATH/theiactl
```

On Mac:

```bash
make theiactl-darwin
chmod +x ./bin/theiactl
mv ./bin/theiactl /some-dir-in-your-PATH/theiactl
```

## Usage

To see the list of available commands and options, run `theiactl help`. Currently, we
have 3 commands for the policy recommendation feature:

- `theiactl policyreco start`
- `theiactl policyreco check`
- `theiactl policyreco result`

For details, please refer to [network policy recommendation doc](./network-policy-recommendation.md)
