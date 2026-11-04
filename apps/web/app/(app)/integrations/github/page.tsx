import { PageHeader } from "@/components/ui";

export const metadata = {
  title: "Connect GitHub · Atlas",
  description: "Link your GitHub account to Atlas.",
};

export default function ConnectGitHubPage() {
  return (
    <div className="max-w-3xl space-y-6">
      <PageHeader title="Connect GitHub" description="Link your GitHub account to Atlas." />
      <div className="p-[22px] text-[12.5px] text-muted">GitHub connection page.</div>
    </div>
  );
}
