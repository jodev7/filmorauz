"use client";

interface Props {
  src: string;
  alt: string;
  className?: string;
}

export default function MoviePoster({ src, alt, className }: Props) {
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={alt}
      className={className}
      onError={(e) => {
        (e.target as HTMLImageElement).src = "/placeholder-poster.jpg";
      }}
    />
  );
}
