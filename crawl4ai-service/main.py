from fastapi import FastAPI
from pydantic import BaseModel
from crawl4ai import AsyncWebCrawler

app = FastAPI()


class CrawlRequest(BaseModel):
    url: str


class CrawlResponse(BaseModel):
    success: bool
    markdown: str = ""
    error: str = ""


@app.post("/crawl", response_model=CrawlResponse)
async def crawl(req: CrawlRequest) -> CrawlResponse:
    try:
        async with AsyncWebCrawler() as crawler:
            result = await crawler.arun(url=req.url)
            if not result.success:
                return CrawlResponse(success=False, error=str(getattr(result, "error_message", "crawl failed")))
            markdown = result.markdown
            # crawl4ai's `markdown` field has varied in shape across
            # versions (plain string vs. an object with a `.raw_markdown`
            # attribute) — handle both.
            if not isinstance(markdown, str):
                markdown = getattr(markdown, "raw_markdown", str(markdown))
            return CrawlResponse(success=True, markdown=markdown or "")
    except Exception as e:
        return CrawlResponse(success=False, error=str(e))


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}
