export type IdLike = string | number;

export function hasId(value: unknown): value is IdLike {
  return value !== null && value !== undefined && String(value).trim().length > 0;
}

export function toIdString(value: unknown) {
  return String(value);
}

export function toApiIdParam(value: unknown): API.Int64 {
  return String(value);
}

export function toApiIdList(values: ReadonlyArray<unknown>): API.Int64[] {
  return values.filter(hasId).map((value) => toApiIdParam(value));
}
