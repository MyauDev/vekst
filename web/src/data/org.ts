/**
 * Creating an organisation: the one call a signed-in caller with no
 * membership may make (`OrgService.CreateOrganization`).
 *
 * Real from the start, shaped like the other three real modules
 * (`dedup.ts`, `importUpload.ts`) rather than the fixture layer's -- there
 * was never a fixture for this screen, since it did not exist before there
 * was a backend to call.
 */
import { ConnectError, createClient } from "@connectrpc/connect";

import { transport } from "../transport";
import { OrgService } from "../gen/vekst/v1/org_pb";

const client = createClient(OrgService, transport);

export interface CreatedOrganisation {
  organizationId: string;
  entityId: string;
}

/**
 * A coded failure from `CreateOrganization` -- `code` is one of
 * `org_name_required`, `org_entity_name_required`, `org_unsupported_country`,
 * `org_unsupported_currency` or `org_already_a_member`.
 *
 * Decoded here, not in `FirstRunScreen`: a screen must never import
 * `@connectrpc/connect` (`data.test.ts`'s seam test), so unwrapping a
 * `ConnectError` into a plain code is this module's job.
 */
export class CreateOrganisationError extends Error {
  constructor(readonly code: string) {
    super(code);
    this.name = "CreateOrganisationError";
  }
}

export async function createOrganisation(input: {
  name: string;
  country: string;
  baseCurrency: string;
  entityName: string;
}): Promise<CreatedOrganisation> {
  try {
    const res = await client.createOrganization({
      name: input.name,
      country: input.country,
      baseCurrency: input.baseCurrency,
      entityName: input.entityName,
    });
    return { organizationId: res.organizationId, entityId: res.entityId };
  } catch (err) {
    throw new CreateOrganisationError(ConnectError.from(err).rawMessage || "error.unknown");
  }
}
