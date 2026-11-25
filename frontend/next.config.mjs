/** @type {import('next').NextConfig} */
const api = process.env.API_URL || "http://localhost:8080";
export default {
  output: "standalone",
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${api}/api/:path*` }];
  },
};
