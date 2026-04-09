export interface Entry {
  id: string;
  label: string;
  value: string;
  order: number;
  createdAt: string; // ISO 8601 UTC
  updatedAt: string; // ISO 8601 UTC
}

export type ModalState =
  | null
  | { kind: "create" }
  | { kind: "edit"; entry: Entry }
  | { kind: "delete"; entry: Entry };
