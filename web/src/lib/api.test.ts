import { describe, expect, test, vi } from "vitest";
import { setToken, uploadTus, resumeUploadTus, type UploadProgress } from "./api";

class FakeXHR {
  upload = {
    onprogress: null as ((event: ProgressEvent) => void) | null
  };
  status = 204;
  statusText = "No Content";
  responseText = "";
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  method = "";
  location = "";
  body: BodyInit | null = null;
  headers = new Map<string, string>();

  open(method: string, location: string) {
    this.method = method;
    this.location = location;
  }

  setRequestHeader(key: string, value: string) {
    this.headers.set(key, value);
  }

  getResponseHeader(key: string) {
    return key === "Upload-Final-Path" ? "/target/photo.jpg" : null;
  }

  send(body: BodyInit) {
    this.body = body;
    this.upload.onprogress?.({ loaded: 5, total: 10, lengthComputable: true } as ProgressEvent);
    this.upload.onprogress?.({ loaded: 10, total: 10, lengthComputable: true } as ProgressEvent);
    this.onload?.();
  }
}

describe("uploadTus", () => {
  test("creates a TUS upload and reports patch progress", async () => {
    setToken("test-token");
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, {
      status: 201,
      headers: {
        Location: "/api/tus/upload-1"
      }
    }));
    const xhr = new FakeXHR();
    const progress: UploadProgress[] = [];
    const file = new File(["0123456789"], "photo.jpg", { type: "image/jpeg" });

    const finalPath = await uploadTus(file, "/target", event => progress.push(event), {
      fetch: fetchMock as unknown as typeof fetch,
      xhrFactory: () => xhr as unknown as XMLHttpRequest
    });

    expect(finalPath).toBe("/target/photo.jpg");
    expect(fetchMock).toHaveBeenCalledWith("/api/tus?path=%2Ftarget", expect.objectContaining({
      method: "POST"
    }));
    const createHeaders = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(createHeaders.Authorization).toBe("Bearer test-token");
    expect(createHeaders["Tus-Resumable"]).toBe("1.0.0");
    expect(createHeaders["Upload-Length"]).toBe("10");
    expect(createHeaders["Upload-Metadata"]).toBe("filename cGhvdG8uanBn");
    expect(xhr.method).toBe("PATCH");
    expect(xhr.location).toBe("/api/tus/upload-1");
    expect(xhr.headers.get("Authorization")).toBe("Bearer test-token");
    expect(xhr.headers.get("Upload-Offset")).toBe("0");
    expect(progress.map(event => event.percent)).toEqual([50, 99, 100]);
  });

  test("calls onUploadCreated with the TUS URL", async () => {
    setToken("test-token");
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, {
      status: 201,
      headers: { Location: "/api/tus/upload-99" }
    }));
    const xhr = new FakeXHR();
    let capturedUrl = "";

    await uploadTus(new File(["data"], "f.bin"), "/", undefined, {
      fetch: fetchMock as unknown as typeof fetch,
      xhrFactory: () => xhr as unknown as XMLHttpRequest,
      onUploadCreated: url => { capturedUrl = url; }
    });

    expect(capturedUrl).toBe("/api/tus/upload-99");
  });

  test("surfaces server create errors", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ error: "invalid filename" }), {
      status: 400,
      headers: { "Content-Type": "application/json" }
    }));

    await expect(uploadTus(new File(["x"], "bad/name.jpg"), "/", undefined, {
      fetch: fetchMock as unknown as typeof fetch
    })).rejects.toThrow("invalid filename");
  });
});

describe("resumeUploadTus", () => {
  test("resumes from server offset and reports progress correctly", async () => {
    setToken("test-token");
    const file = new File(["0123456789"], "photo.jpg", { type: "image/jpeg" });
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, {
      status: 204,
      headers: { "Upload-Offset": "5" }
    }));
    const progress: UploadProgress[] = [];

    const xhr = new FakeXHR();
    // Simulate uploading the remaining 5 bytes (5..10).
    const origSend = xhr.send.bind(xhr);
    xhr.send = function(body: BodyInit) {
      this.body = body;
      this.upload.onprogress?.({ loaded: 3, total: 5, lengthComputable: true } as ProgressEvent);
      this.upload.onprogress?.({ loaded: 5, total: 5, lengthComputable: true } as ProgressEvent);
      this.onload?.();
    };
    void origSend;

    await resumeUploadTus("/api/tus/upload-1", file, event => progress.push(event), {
      fetch: fetchMock as unknown as typeof fetch,
      xhrFactory: () => xhr as unknown as XMLHttpRequest
    });

    expect(xhr.headers.get("Upload-Offset")).toBe("5");
    // Progress percents should reflect overall file progress (startOffset=5, total=10).
    // loaded=3+5=8 of 10 → 80%, loaded=5+5=10 of 10 → 99 (capped), then 100.
    expect(progress.map(e => e.percent)).toEqual([80, 99, 100]);
  });

  test("throws upload_gone when server returns 404", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, { status: 404 }));

    await expect(resumeUploadTus("/api/tus/gone", new File(["x"], "f.bin"), undefined, {
      fetch: fetchMock as unknown as typeof fetch
    })).rejects.toThrow("upload_gone");
  });

  test("returns immediately when server offset equals file size", async () => {
    const file = new File(["0123456789"], "done.bin");
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, {
      status: 204,
      headers: { "Upload-Offset": "10" }
    }));
    const xhrFactory = vi.fn();

    const finalPath = await resumeUploadTus("/api/tus/upload-done", file, undefined, {
      fetch: fetchMock as unknown as typeof fetch,
      xhrFactory: xhrFactory as unknown as () => XMLHttpRequest
    });

    expect(xhrFactory).not.toHaveBeenCalled();
    expect(finalPath).toBe("");
  });
});
