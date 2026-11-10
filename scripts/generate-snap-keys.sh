#!/usr/bin/env bash
# Generates a 2048-bit RSA key pair for Kirim DOKU SNAP authentication (Bank Indonesia standard).
# The private key must be kept secret and configured in DOKU_SNAP_PRIVATE_KEY.
# The public key must be uploaded / registered in the DOKU Dashboard (Sandbox & Production).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/../certs"
mkdir -p "${OUTPUT_DIR}"

PRIVATE_KEY="${OUTPUT_DIR}/snap-private.pem"
PUBLIC_KEY="${OUTPUT_DIR}/snap-public.pem"

echo "==> Generating 2048-bit RSA key pair for DOKU SNAP..."
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "${PRIVATE_KEY}"
openssl rsa -pubout -in "${PRIVATE_KEY}" -out "${PUBLIC_KEY}"

echo ""
echo "Keys generated successfully:"
echo "  Private key: ${PRIVATE_KEY}"
echo "  Public key:  ${PUBLIC_KEY}"
echo ""
echo "==> ACTION REQUIRED IN DOKU DASHBOARD:"
echo "1. Login to https://sandbox.doku.com (or production)"
echo "2. Navigate to Settings > API Credentials / SNAP BI"
echo "3. Register your Public Key by pasting the content below:"
echo "----------------------------------------------------------------"
cat "${PUBLIC_KEY}"
echo "----------------------------------------------------------------"
echo ""
echo "4. In serabutin-id-backend/.env, configure:"
echo "   DOKU_SNAP_PRIVATE_KEY=\"\$(base64 -w0 ${PRIVATE_KEY})\""
echo "   (or provide the path ${PRIVATE_KEY})"
