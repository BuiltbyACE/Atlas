import { notFound } from "next/navigation";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import { listDocuments, type Document } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";

/**
 * Document library (§8.6). List of stored documents, filterable by category.
 * Each row links to a download URL.
 */
export default async function DocumentsPage() {
  const user = await requireUser();

  let documents: Document[] = [];
  let error: ApiError | null = null;
  try {
    documents = await listDocuments();
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Documents" description="Document library." />
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view documents"
              : "Documents could not be loaded"
          }
        />
      </div>
    );
  }

  const categoryTone: Record<string, "neutral" | "success" | "warning" | "danger" | "info"> =
    { invoice: "success", system_doc: "info", contract: "neutral", policy: "neutral", other: "neutral" };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Documents"
        description="Document library — invoices, system docs, contracts, policies."
        actions={
          <ButtonLink href="/documents/upload" variant="primary">
            <Icon name="upload" className="h-4 w-4" />
            Upload
          </ButtonLink>
        }
      />

      {documents.length === 0 ? (
        <div className="p-[22px]">
          <EmptyState
            icon="document"
            iconTone="info"
            className="min-h-[240px]"
            title="No documents"
          >
            Upload your first document. Invoices, system documentation,
            contracts and policies are all supported.
          </EmptyState>
        </div>
      ) : (
        <Card>
          <Table>
            <thead>
              <tr>
                <Th>Document</Th>
                <Th>Category</Th>
                <Th className="text-right">Size</Th>
                <Th className="text-right">Uploaded</Th>
                <Th>Uploader</Th>
              </tr>
            </thead>
            <tbody>
              {documents.map((doc) => (
                <tr key={doc.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <a
                        href={`/documents/${doc.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {doc.filename}
                      </a>
                    </div>
                  </Td>
                  <Td>
                    <Badge tone={categoryTone[doc.category] ?? "neutral"}>
                      {humanise(doc.category)}
                    </Badge>
                  </Td>
                  <Td className="tabular text-right text-muted">
                    {(doc.size_bytes / 1024).toFixed(1)} KB
                  </Td>
                  <Td className="text-muted">{formatDay(doc.uploaded_at)}</Td>
                  <Td className="text-muted">{doc.uploaded_by.slice(0, 8)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card>
      )}
    </div>
  );
}
