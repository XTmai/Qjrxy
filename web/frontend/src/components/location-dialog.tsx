// 签到位置填写：不依赖任何地图 SDK / key。
// 用户用高德官方坐标拾取页取点后把「经度,纬度」粘贴回来；
// 也可以直接从学校下发的查寝任务里读取签到点。
import { useState } from "react";
import { ExternalLink, Loader2, MapPin } from "lucide-react";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { api } from "@/api";
import type { Account } from "@/lib/types";

const AMAP_PICKER_URL = "https://www.amap.com/ssr/coordinate-picker";

function isValidCoord(lon: number, lat: number): boolean {
  return (
    Number.isFinite(lon) &&
    Number.isFinite(lat) &&
    Math.abs(lon) <= 180 &&
    Math.abs(lat) <= 90
  );
}

/**
 * 从一段文本里解析经纬度。
 *
 * 兼容：高德拾取页的「经度,纬度」、空格/分号分隔、带标签的文案、
 * 以及带 lng/lat 参数的分享链接。高德固定经度在前；若数字明显是
 * 「纬度,经度」（第一个在 ±90 内、第二个超出 ±90），自动纠正。
 */
export function parseCoordinateText(raw: string): { lon: number; lat: number } | null {
  const text = raw.trim();
  if (!text) return null;

  // 分享链接：?lng=116.397428&lat=39.909230
  if (text.includes("?")) {
    const params = new URLSearchParams(text.slice(text.indexOf("?") + 1));
    const lonRaw = params.get("lng") ?? params.get("lon") ?? params.get("longitude");
    const latRaw = params.get("lat") ?? params.get("latitude");
    if (lonRaw && latRaw) {
      const lon = Number(lonRaw);
      const lat = Number(latRaw);
      if (isValidCoord(lon, lat)) return { lon, lat };
    }
  }

  const numbers = text.match(/-?\d+(?:\.\d+)?/g);
  if (!numbers || numbers.length < 2) return null;
  let lon = Number(numbers[0]);
  let lat = Number(numbers[1]);
  if (Math.abs(lon) <= 90 && Math.abs(lat) > 90) {
    [lon, lat] = [lat, lon];
  }
  if (!isValidCoord(lon, lat)) return null;
  return { lon, lat };
}

export function LocationDialog({
  account,
  onClose,
}: {
  account: Account;
  onClose: (saved: boolean) => void;
}) {
  const [lon, setLon] = useState(account.lon ? account.lon.toFixed(6) : "");
  const [lat, setLat] = useState(account.lat ? account.lat.toFixed(6) : "");
  const [address, setAddress] = useState(account.address || "");
  const [paste, setPaste] = useState("");
  const [busy, setBusy] = useState(false);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState("");
  const [hint, setHint] = useState("");

  const fillCoord = (nextLon: number, nextLat: number, message?: string) => {
    setLon(nextLon.toFixed(6));
    setLat(nextLat.toFixed(6));
    setError("");
    setHint(message ?? "坐标已填入，确认无误后保存");
  };

  const applyPaste = () => {
    const parsed = parseCoordinateText(paste);
    if (!parsed) {
      setError("没有识别到有效坐标。请粘贴「经度,纬度」，例如 116.397428,39.909230");
      return;
    }
    fillCoord(parsed.lon, parsed.lat);
    setPaste("");
  };

  // 粘贴时能直接识别就自动填入，省一次点击；识别不了则保留文本让用户手动处理。
  const onPaste = (text: string, prevent: () => void) => {
    const parsed = parseCoordinateText(text);
    if (!parsed) return;
    prevent();
    fillCoord(parsed.lon, parsed.lat);
    setPaste("");
  };

  const readFromSchool = async () => {
    setReading(true);
    setError("");
    setHint("");
    try {
      const { range } = await api.signRange(account.id);
      fillCoord(
        range.lon,
        range.lat,
        `已从「${range.taskName}」读取签到点${range.radius ? `，半径约 ${Math.round(range.radius)} 米` : ""}，确认后保存`,
      );
      if (range.address) setAddress(range.address);
    } catch (e) {
      setError(e instanceof Error ? e.message : "读取失败，请确认账号已登录且当前有查寝任务");
    } finally {
      setReading(false);
    }
  };

  const save = async () => {
    setError("");
    if (!lon.trim() || !lat.trim()) {
      setError("请填写经度和纬度，或先从学校读取签到点");
      return;
    }
    const lonNum = Number(lon);
    const latNum = Number(lat);
    if (!isValidCoord(lonNum, latNum)) {
      setError("经纬度不正确：经度 -180 ~ 180，纬度 -90 ~ 90");
      return;
    }
    const outsideChina = lonNum < 73 || lonNum > 136 || latNum < 3 || latNum > 54;
    if (outsideChina) {
      setError("这个坐标不在中国境内，请确认没有把经纬度填反或选错点");
      return;
    }

    setBusy(true);
    try {
      await api.updateAccount(account.id, {
        address: address.trim(),
        lon: lonNum,
        lat: latNum,
      });
      onClose(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose(false)}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>设置签到位置</DialogTitle>
          <DialogDescription className="truncate">
            {account.alias || account.username} · 提交查寝时使用这组坐标
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          {/* 取点指引：不内置地图，避免开源项目绑定地图 key */}
          <div className="bg-muted/50 border-border space-y-1.5 rounded-lg border p-3">
            <p className="text-xs font-medium">不知道坐标？用高德地图拾取</p>
            <ol className="text-muted-foreground list-decimal space-y-0.5 pl-4 text-[11px]">
              <li>
                打开{" "}
                <a
                  href={AMAP_PICKER_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-brand inline-flex items-center gap-0.5 underline underline-offset-2"
                >
                  高德坐标拾取页
                  <ExternalLink className="size-3" />
                </a>
              </li>
              <li>搜索或缩放到宿舍楼，点击地图上的点</li>
              <li>复制「经度,纬度」，粘贴到下面</li>
            </ol>
            <p className="text-muted-foreground text-[11px]">
              拾取页返回的是 GCJ-02 坐标，与国内校园 App 常用坐标系一致。不要使用
              Google/OSM 的 WGS-84 坐标，否则会有几百米偏移。
            </p>
          </div>

          <div className="flex gap-2">
            <Input
              value={paste}
              onChange={(e) => setPaste(e.target.value)}
              onPaste={(e) => onPaste(e.clipboardData.getData("text"), () => e.preventDefault())}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  applyPaste();
                }
              }}
              placeholder="粘贴坐标：116.397428,39.909230"
              autoComplete="off"
            />
            <Button variant="outline" onClick={applyPaste} disabled={!paste.trim()}>
              识别
            </Button>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="loc-lon">经度</FieldLabel>
              <Input
                id="loc-lon"
                value={lon}
                onChange={(e) => setLon(e.target.value)}
                placeholder="116.397428"
                inputMode="decimal"
                autoComplete="off"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="loc-lat">纬度</FieldLabel>
              <Input
                id="loc-lat"
                value={lat}
                onChange={(e) => setLat(e.target.value)}
                placeholder="39.909230"
                inputMode="decimal"
                autoComplete="off"
              />
            </Field>
          </div>

          <Field>
            <FieldLabel htmlFor="loc-address">位置名称（可选）</FieldLabel>
            <Input
              id="loc-address"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="如：XX大学 3 号宿舍楼"
              autoComplete="off"
            />
            <FieldDescription>仅用于界面显示，不影响提交。</FieldDescription>
          </Field>

          <div className="flex items-center justify-between gap-2">
            <Button variant="ghost" size="sm" onClick={() => void readFromSchool()} disabled={reading}>
              {reading ? <Loader2 className="size-3.5 animate-spin" /> : <MapPin className="size-3.5" />}
              从学校读取签到点
            </Button>
            <span className="text-muted-foreground text-[11px]">学校下发的最准确</span>
          </div>

          {error && <p className="text-destructive text-xs">{error}</p>}
          {!error && hint && <p className="text-muted-foreground text-xs">{hint}</p>}
        </DialogBody>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(false)} disabled={busy}>
            取消
          </Button>
          <Button onClick={() => void save()} disabled={busy || reading}>
            {busy && <Loader2 className="size-3.5 animate-spin" />}
            保存位置
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
