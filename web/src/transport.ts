import { createConnectTransport } from "@connectrpc/connect-web";

/**
 * The browser API transport.
 *
 * The base URL is relative on purpose. In every environment the Ingress routes
 * `/rpc/*` to core and everything else to the web app, so the document and the
 * RPC share an origin and no CORS header is ever required. An absolute URL here
 * would make the deployed environment differ from the local one in exactly the
 * way design D6 exists to prevent.
 */
export const transport = createConnectTransport({ baseUrl: "/rpc" });
