import { twc } from "react-twc";
import { useLogQuery } from "../api/queries.ts";
import { Button, Card, CardBody, CardHeader, CardTitle } from "../components/ui/index.ts";

const PageTitle = twc.h1`text-xl font-semibold text-gray-900 mb-6`;

const PageRow = twc.div`flex items-center justify-between mb-6`;

const LogPre = twc.pre`
  text-xs font-mono text-gray-800 whitespace-pre-wrap break-all
  max-h-[60vh] overflow-y-auto
`;

const EmptyNote = twc.p`text-sm text-gray-400 text-center py-8`;

export function LogPage() {
  const logQuery = useLogQuery();
  const lines = logQuery.data ?? [];
  const loading = logQuery.isFetching;

  return (
    <>
      <PageRow>
        <PageTitle>系统日志</PageTitle>
        <Button
          $variant="secondary"
          $size="sm"
          onClick={() => void logQuery.refetch()}
          disabled={loading}
        >
          {loading ? "加载中…" : "刷新"}
        </Button>
      </PageRow>

      <Card>
        <CardHeader>
          <CardTitle>journald 日志（最近）</CardTitle>
        </CardHeader>
        <CardBody>
          {lines.length === 0 ? (
            <EmptyNote>暂无日志数据</EmptyNote>
          ) : (
            <LogPre>{lines.join("\n")}</LogPre>
          )}
        </CardBody>
      </Card>
    </>
  );
}
