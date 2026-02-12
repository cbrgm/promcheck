# Security Policy

## Supported Versions

Only the latest release is actively supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < latest | :x:               |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Send an email to **github@cbrgm.net** with:
   - A description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Any suggested fixes (optional)

You can expect:
- An acknowledgment within 48 hours (at least I try to, since I'm doing this in my freetime)
- Regular updates on the progress
- Credit in the security advisory (unless you prefer to remain anonymous)

## Verifying Release Signatures

All release binaries are signed with PGP. To verify a release:

### 1. Import the Public Key

```bash
gpg --import <<'EOF'
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBGmNIX8BEACwa36sCFVDDnIVnNZHqLYKoj0eZn1SoWXhVQKxR1QYniFeq8+u
5KZ/5INXhWzUT8jpoh/2lzxBg6IT8dTXdmZ1vgM0s3KtJp+uNPmmggYlx69rwtVi
PE0JOeCMtlbM70SCVM86eHA9gdXyXqBT1OERdkq1EfNshWz4MFJWDXNcx5VksHpY
MJHrSfENOIjnYOjyI88OEtIs1XSKCEFbWfNh//Tbk4DsO07wwi5G2QLFRt1rZM3d
SHNAo+s+qDc3oLZCQ5KBPKpPETO9PgwCfaRLQ1TGSTyQUr3Ok06rpdjyp9OD4bw5
Vs4apWFNiJ7xkofbyOoi5zrrkrQMWhHhrCHYcuvvVvrHrsAVcv2t+nzXbKy7GXcO
xT3xeggT7ohmNR7QBn7flKHvucVj6HI+879l7U0KoX/m4gvWNQufHu9mYFCiW9C3
L4P3onf503FPQgMhptt3R+OQLQcnFDu+78lpVNQDNeRwc4XqedUEGFpBhzO3DQJO
dHshlty6GSMtG+N50dpjpetmFg4RG/fI4sdyYAZoiXyfcnzXu9Q4QVMmhr9QgnWs
Mf9skQ54sPLwLk5xBjrQK7HE0Mr5aUTCMtybVYApb9Ft1XJ2Z9FpKz2mNnbw9HXv
/8i+WQ2i3yvC5PN0pPezZBSgDf3r9J14sHUqDiHSZt84LQ1T9meirl2iXQARAQAB
tCVDaHJpc3RpYW4gQmFyZ21hbm4gPGdpdGh1YkBjYnJnbS5uZXQ+iQJRBBMBCAA7
FiEE3iykF4aUXFzBL19DXx8YVj5HZmYFAmmNIX8CGwMFCwkIBwICIgIGFQoJCAsC
BBYCAwECHgcCF4AACgkQXx8YVj5HZmYomxAAiUof2u9yvxeJXy6TNYIpcJqvsKw4
++DJ5m+PH0v8qBATYvYqqpN3CNG7OyoM4OM8ziBVwwSno0A7tB7fE6vzBi/Znij9
Sni6neJk48FL2mTDI+Z/RHOf1b2zGFKgnS7V2IpTWXbMW/OKzSy5xxgOIg9VEdRT
rJGPqquSoRf12YCQe76aNibMRIeK9Npq/DbVAtD7g+g1sbNI9rkuFEav+ehAXtNk
hPexwG0k2hILwG2ljKJ4mCocvIl5jR7e+fnrp0E7COYP0qH7Ou8mTowPjIozlG7J
5ZNuIMVW1KAQtM4n0UIHMG72uB0G30oMA5WZIXCI3NqxudlznTmoJgGrHIrKRMu7
TT6w2rLG6xTdX96Q/boeIBOHI2riQVEN55pswPoXBTTadFzKQ4bQnot4JXXs0oBj
L4DfLhC0bBOnGSFBoEeTqrjHmSszdWjEi6hEskMcKqqXDnsQj2p9E0axXtYbU8JF
NZSoZ+b9M+2bqzq2JsDZJBJocmTkXH13mZYtmr1wTOVI8TaKe9OuKnv7kOKTLWDf
wslay7US3LuGSziUAb0BIBJWMcZs/ou+W5wT85iLOjkiEvnUx8J5rR137Xl72ZUk
Z46Jit0Zi3BnADqx0Q6pJfDgRlQ/R7adpm6DLTLVl/+LWH5aNy+U7nSBQ+bFimnJ
dnWz3mMJWwgwNeE=
=fBYq
-----END PGP PUBLIC KEY BLOCK-----
EOF
```

Alternatively, fetch the key from a keyserver:

```bash
gpg --keyserver keys.openpgp.org --recv-keys DE2CA41786945C5CC12F5F435F1F18563E476666
```

### 2. Verify the Key Fingerprint

Ensure the key fingerprint matches:

```
pub   rsa4096 2026-02-12 [SC]
      DE2C A417 8694 5C5C C12F  5F43 5F1F 1856 3E47 6666
uid           [ultimate] Christian Bargmann <github@cbrgm.net>
```

### 3. Verify a Release

```bash
# Verify the signature
gpg --verify <binary>.asc <binary>

# Verify the checksum
sha256sum -c <binary>.sha256
```

## Security Best Practices

When using this project:

- Always verify release signatures before deploying
- Keep dependencies up to date
- Review the changelog before upgrading
- Pin to specific versions in production environments (!)
