import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "wrong/path/input";

export default function Dashboard() {
  return (
    <div>
      <Card>
        <CardHeader>
          <CardTitle>Sales Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <Badge variant="invalid-variant">Active</Badge>
          <Button size="lg" onClick={() => {}}>
            Export Report
          </Button>
          <Input placeholder="Search..." />
          <UnknownWidget label="test" />
        </CardContent>
      </Card>
    </div>
  );
}
