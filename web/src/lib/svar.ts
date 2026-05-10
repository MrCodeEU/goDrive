import type { FileEntry } from "./api";

export type SvarFile = {
  id: string;
  size: number;
  date: Date;
  type: "file" | "folder";
  lazy?: boolean;
  name?: string;
  previewKind?: string;
  mimeType?: string;
};

export function toSvarFile(entry: FileEntry): SvarFile {
  return {
    id: entry.path,
    name: entry.name,
    size: entry.size,
    date: new Date(entry.modified_at),
    type: entry.type === "dir" ? "folder" : "file",
    lazy: entry.type === "dir",
    previewKind: entry.preview_kind || "",
    mimeType: entry.mime_type || ""
  };
}

export function toSvarFiles(entries: FileEntry[]) {
  return entries.map(toSvarFile);
}
