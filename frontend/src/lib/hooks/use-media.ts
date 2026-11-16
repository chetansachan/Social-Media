"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import {
  isSignatureExpired,
  mediaKindOfFile,
  mediaLimitError,
  resourceTypeFor,
  uploadToCloudinary,
} from "@/lib/media";
import type { MediaKind, UploadSignature } from "@/lib/types";

export const uploadKeys = {
  signature: (resourceType: string) => ["upload-signature", resourceType] as const,
};

/** Signatures live ten minutes server-side; stop reusing one nine minutes in. */
const SIGNATURE_STALE_MS = 9 * 60_000;

/**
 * A usable Cloudinary signature for one resource type.
 *
 * Cached rather than fetched per upload, because the signature is good for ten
 * minutes and attaching media to a post should not ask post-service twice. An
 * expired entry is refetched instead of reused: Cloudinary validates at
 * completion time, so a stale signature fails after the whole file has been
 * sent, which is the most expensive way to learn it was stale.
 *
 * A failure is not cached — `throwOnError` is off and the retry default applies.
 */
async function loadSignature(
  queryClient: ReturnType<typeof useQueryClient>,
  resourceType: "image" | "video",
): Promise<UploadSignature> {
  const key = uploadKeys.signature(resourceType);

  const cached = queryClient.getQueryData<UploadSignature>(key);
  if (cached && !isSignatureExpired(cached)) return cached;

  return queryClient.fetchQuery({
    queryKey: key,
    staleTime: SIGNATURE_STALE_MS,
    queryFn: async ({ signal }) => {
      const data = await gql<{ getUploadSignature: UploadSignature }>(
        operations.uploadSignature,
        { resourceType, folder: null },
        signal,
      );
      return data.getUploadSignature;
    },
  });
}

/**
 * Uploads one picked file and returns its Cloudinary URL.
 *
 * One call is the whole transaction: request a signature if the cached one is
 * stale, post the file straight to Cloudinary, hand the URL back for the
 * composer to store as the post's `mediaUrl`.
 */
export function useMediaUpload() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ file, kind }: { file: File; kind: MediaKind }) => {
      // The kind is re-derived from the file, not trusted from the caller: the
      // composer computed it once and the user could have swapped the file since.
      const actual = mediaKindOfFile(file);
      if (!actual) {
        throw new Error(`${file.name || "That file"} is not an image or video.`);
      }
      if (actual !== kind) {
        throw new Error("The attached file changed. Please pick it again.");
      }

      const limitProblem = mediaLimitError(file, actual);
      if (limitProblem) throw new Error(limitProblem);

      const signature = await loadSignature(queryClient, resourceTypeFor(actual));
      return uploadToCloudinary(file, actual, signature);
    },
  });
}