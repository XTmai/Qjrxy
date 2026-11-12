// 照片池：每个账号可上传多张签到照片，执行时按上传顺序轮换。
// 池不为空时优先使用池内照片，池为空/文件失效时回退到单张兜底照片。
import { useRef, useState } from "react";
import { ImageIcon, Loader2, Trash2, Upload } from "lucide-react";

import { api } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/hooks/use-toast";
import type { Account, AccountPhoto } from "@/lib/types";

const MAX_POOL = 10;
const MAX_BATCH = 10;
const MAX_PHOTO_BYTES = 5 * 1024 * 1024;

interface Props {
  account: Account;
  photos: AccountPhoto[];
  loading: boolean;
  onReload: () => Promise<void>;
  onChanged: () => void;
}

export function PhotoTab({ account, photos, loading, onReload, onChanged }: Props) {
  const { toast } = useToast();
  const { confirm, ConfirmDialog } = useConfirm();
  const poolRef = useRef<HTMLInputElement>(null);
  const fallbackRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [photoVersion, setPhotoVersion] = useState(Date.now());

  // 兜底照片只认单张（account.photoPath）；photoUrl 在有池无单张时会指向池封面。
  const hasFallback = Boolean(account.photoPath);

  const uploadPool = async (picked: FileList) => {
    const files = Array.from(picked);
    if (files.length === 0) return;
    if (files.length > MAX_BATCH) {
      toast({ title: `单次最多上传 ${MAX_BATCH} 张`, variant: "destructive" });
      return;
    }
    if (photos.length + files.length > MAX_POOL) {
      toast({
        title: "照片池最多 10 张",
        description: `当前 ${photos.length} 张，本次选择 ${files.length} 张`,
        variant: "destructive",
      });
      return;
    }
    const oversized = files.find((f) => f.size > MAX_PHOTO_BYTES);
    if (oversized) {
      toast({
        title: `${oversized.name} 超过 5MB`,
        description: "请先压缩或换一张更小的照片",
        variant: "destructive",
      });
      return;
    }
    setBusy(true);
    try {
      const fd = new FormData();
      for (const file of files) fd.append("photos", file);
      const res = await api.uploadAccountPhotos(account.id, fd);
      await onReload();
      onChanged();
      if (res.failed.length > 0) {
        toast({
          title: `已上传 ${res.photos.length} 张，${res.failed.length} 张失败`,
          description: `${res.failed[0].name}：${res.failed[0].error}`,
          variant: "destructive",
        });
      } else {
        toast({ title: `已上传 ${res.photos.length} 张照片`, description: "执行查寝时按上传顺序轮换使用" });
      }
    } catch (e) {
      toast({ title: "上传失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const uploadFallback = async (file: File) => {
    if (file.size > MAX_PHOTO_BYTES) {
      toast({ title: "照片超过 5MB", description: "请先压缩或换一张更小的照片", variant: "destructive" });
      return;
    }
    setBusy(true);
    try {
      const fd = new FormData();
      fd.append("photo", file);
      await api.uploadPhoto(account.id, fd);
      setPhotoVersion(Date.now());
      onChanged();
      toast({ title: "兜底照片已保存" });
    } catch (e) {
      toast({ title: "上传失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const removePhoto = async (photo: AccountPhoto, index: number) => {
    const ok = await confirm({
      title: "删除照片",
      description: `确定从照片池移除第 ${index + 1} 张照片吗？删除后不可恢复。`,
      confirmText: "删除",
      variant: "destructive",
    });
    if (!ok) return;
    setBusy(true);
    try {
      await api.deleteAccountPhoto(account.id, photo.id);
      await onReload();
      onChanged();
      toast({ title: "照片已删除" });
    } catch (e) {
      toast({ title: "删除失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3 py-3">
      <input
        ref={poolRef}
        type="file"
        multiple
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(e) => {
          const files = e.target.files;
          if (files && files.length > 0) void uploadPool(files);
          e.target.value = "";
        }}
      />
      <input
        ref={fallbackRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void uploadFallback(f);
          e.target.value = "";
        }}
      />

      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="flex items-center gap-2 text-sm font-medium">
            照片池
            <Badge variant="secondary" className="tabular-nums">
              {photos.length} 张
            </Badge>
          </p>
          <p className="text-muted-foreground mt-0.5 text-[11px]">
            按上传顺序轮换；池不为空时优先使用，池空才用下方兜底照片。
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          className="shrink-0 gap-1.5"
          disabled={busy}
          onClick={() => poolRef.current?.click()}
        >
          {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
          上传
        </Button>
      </div>

      {loading ? (
        <div className="border-border text-muted-foreground rounded-lg border border-dashed py-8 text-center text-xs">
          正在读取照片…
        </div>
      ) : photos.length === 0 ? (
        <div className="border-border text-muted-foreground rounded-lg border border-dashed px-4 py-8 text-center text-xs">
          <ImageIcon className="text-muted-foreground/50 mx-auto mb-2 size-6" />
          照片池为空，执行查寝时会使用下方兜底照片
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-2">
          {photos.map((photo, index) => (
            <div key={photo.id} className="group border-border relative aspect-square overflow-hidden rounded-lg border">
              <img
                src={`${photo.url}?w=320&t=${encodeURIComponent(photo.createdAt)}`}
                alt={`照片 ${index + 1}`}
                loading="lazy"
                decoding="async"
                className="size-full object-cover"
              />
              <span className="absolute top-1.5 left-1.5 rounded bg-black/60 px-1.5 py-0.5 font-mono text-[10px] text-white">
                #{index + 1}
              </span>
              <button
                type="button"
                disabled={busy}
                onClick={() => void removePhoto(photo, index)}
                aria-label={`删除第 ${index + 1} 张照片`}
                className="absolute top-1.5 right-1.5 flex size-6 items-center justify-center rounded-md bg-black/55 text-white transition-colors hover:bg-destructive disabled:opacity-50"
              >
                <Trash2 className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <p className="text-muted-foreground text-[10px]">最多 10 张 · 单张 ≤ 5MB · 支持 jpg / png / webp</p>

      <div className="border-border space-y-2.5 rounded-lg border p-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs font-medium">兜底照片</p>
            <p className="text-muted-foreground mt-0.5 text-[11px]">池为空或池内文件失效时使用</p>
          </div>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 shrink-0 px-2 text-xs"
            disabled={busy}
            onClick={() => fallbackRef.current?.click()}
          >
            {hasFallback ? "更换" : "上传"}
          </Button>
        </div>
        {hasFallback ? (
          <div className="flex items-center gap-3">
            <img
              src={`${api.photoUrl(account.id)}?w=320&t=${photoVersion}`}
              alt="兜底照片"
              loading="lazy"
              decoding="async"
              className="size-16 shrink-0 rounded-lg border object-cover"
            />
            <p className="text-muted-foreground text-[11px]">查寝提交时优先取池；池空才上传这张。</p>
          </div>
        ) : (
          <p className="text-muted-foreground text-[11px]">未设置。照片池为空时，需要照片的任务会提交失败。</p>
        )}
      </div>

      {ConfirmDialog}
    </div>
  );
}
